package core

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	pkgtun "github.com/wencaiwulue/kubevpn/pkg/tun"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

const (
	MaxSize   = 1024
	MaxThread = 10
)

type tunHandler struct {
	chain  *Chain
	node   *Node
	routes *NAT
	chExit chan error
}

type NAT struct {
	lock   *sync.RWMutex
	routes map[string][]net.Addr
}

func NewNAT() *NAT {
	return &NAT{
		lock:   &sync.RWMutex{},
		routes: map[string][]net.Addr{},
	}
}

func (n *NAT) RemoveAddr(addr net.Addr) (count int) {
	n.lock.Lock()
	defer n.lock.Unlock()
	for k, v := range n.routes {
		for i := 0; i < len(v); i++ {
			if v[i].String() == addr.String() {
				v = append(v[:i], v[i+1:]...)
				i--
				count++
			}
		}
		n.routes[k] = v
	}
	return
}

func (n *NAT) LoadOrStore(to net.IP, addr net.Addr) (result net.Addr, load bool) {
	n.lock.Lock()
	defer n.lock.Unlock()
	addrList := n.routes[to.String()]
	for _, add := range addrList {
		if add.String() == addr.String() {
			load = true
			result = addr
			return
		}
	}

	if addrList == nil {
		n.routes[to.String()] = []net.Addr{addr}
		result = addr
		return
	} else {
		n.routes[to.String()] = append(n.routes[to.String()], addr)
		result = addr
		return
	}
}

func (n *NAT) RouteTo(ip net.IP) net.Addr {
	n.lock.Lock()
	defer n.lock.Unlock()
	addrList := n.routes[ip.String()]
	if len(addrList) == 0 {
		return nil
	}
	// for load balance
	index := rand.Intn(len(n.routes[ip.String()]))
	return addrList[index]
}

func (n *NAT) Remove(ip net.IP, addr net.Addr) {
	n.lock.Lock()
	defer n.lock.Unlock()

	addrList, ok := n.routes[ip.String()]
	if !ok {
		return
	}
	for i := 0; i < len(addrList); i++ {
		if addrList[i].String() == addr.String() {
			addrList = append(addrList[:i], addrList[i+1:]...)
			i--
		}
	}
	n.routes[ip.String()] = addrList
	return
}

func (n *NAT) Range(f func(key string, v []net.Addr)) {
	n.lock.Lock()
	defer n.lock.Unlock()
	for k, v := range n.routes {
		f(k, v)
	}
}

// TunHandler creates a handler for tun tunnel.
func TunHandler(chain *Chain, node *Node) Handler {
	return &tunHandler{
		chain:  chain,
		node:   node,
		routes: RouteNAT,
		chExit: make(chan error, 1),
	}
}

func (h *tunHandler) Handle(ctx context.Context, tun net.Conn) {
	if h.node.Remote != "" {
		h.HandleClient(ctx, tun)
	} else {
		h.HandleServer(ctx, tun)
	}
}

func (h tunHandler) printRoute() {
	for {
		select {
		case <-time.Tick(time.Second * 5):
			var i int
			h.routes.Range(func(key string, value []net.Addr) {
				i++
				var s []string
				for _, addr := range value {
					if addr != nil {
						s = append(s, addr.String())
					}
				}
				fmt.Printf("to: %s, route: %s\n", key, strings.Join(s, " "))
			})
			fmt.Println(i)
		}
	}
}

type Device struct {
	tun    net.Conn
	closed atomic.Bool
	thread int

	tunInboundRaw chan *DataElem
	tunInbound    chan *DataElem
	tunOutbound   chan *DataElem

	chExit chan error
}

func (d *Device) readFromTun() {
	for {
		b := config.LPool.Get().([]byte)
		n, err := d.tun.Read(b[:])
		if err != nil {
			select {
			case d.chExit <- err:
			default:
			}
			return
		}
		if d.closed.Load() {
			return
		}
		d.tunInboundRaw <- &DataElem{
			data:   b[:],
			length: n,
		}
	}
}

func (d *Device) writeToTun() {
	for e := range d.tunOutbound {
		_, err := d.tun.Write(e.data[:e.length])
		config.LPool.Put(e.data[:])
		if err != nil {
			select {
			case d.chExit <- err:
			default:
			}
			return
		}
	}
}

func (d *Device) parseIPHeader() {
	for e := range d.tunInboundRaw {
		if util.IsIPv4(e.data[:e.length]) {
			// ipv4.ParseHeader
			b := e.data[:e.length]
			e.src = net.IPv4(b[12], b[13], b[14], b[15])
			e.dst = net.IPv4(b[16], b[17], b[18], b[19])
		} else if util.IsIPv6(e.data[:e.length]) {
			// ipv6.ParseHeader
			e.src = e.data[:e.length][8:24]
			e.dst = e.data[:e.length][24:40]
		} else {
			log.Errorf("[tun] unknown packet")
			continue
		}

		log.Debugf("[tun] %s --> %s", e.src, e.dst)
		if d.closed.Load() {
			return
		}
		d.tunInbound <- e
	}
}

func (d *Device) Close() {
	d.closed.Store(true)
	d.tun.Close()
	close(d.tunInboundRaw)
	close(d.tunOutbound)
}

func (d *Device) heartbeats() {
	tunIface, err := pkgtun.GetInterface()
	if err != nil {
		return
	}
	addrs, err := tunIface.Addrs()
	if err != nil {
		return
	}
	var srcIPv4, srcIPv6 net.IP
	for _, addr := range addrs {
		ip, cidr, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		if cidr.Contains(config.RouterIP) {
			srcIPv4 = ip
		}
		if cidr.Contains(config.RouterIP6) {
			srcIPv6 = ip
		}
	}
	if srcIPv4 == nil || srcIPv6 == nil {
		return
	}
	if config.RouterIP.To4().Equal(srcIPv4) {
		return
	}
	if config.RouterIP6.To4().Equal(srcIPv6) {
		return
	}

	var bytes []byte
	var bytes6 []byte

	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()

	for ; true; <-ticker.C {
		for i := 0; i < 4; i++ {
			if bytes == nil {
				bytes, err = genICMPPacket(srcIPv4, config.RouterIP)
				if err != nil {
					log.Error(err)
					continue
				}
			}
			if bytes6 == nil {
				bytes6, err = genICMPPacketIPv6(srcIPv6, config.RouterIP6)
				if err != nil {
					log.Error(err)
					continue
				}
			}
			for index, i2 := range [][]byte{bytes, bytes6} {
				if d.closed.Load() {
					return
				}

				data := config.LPool.Get().([]byte)[:]
				length := copy(data, i2)
				var src, dst net.IP
				if index == 0 {
					src, dst = srcIPv4, config.RouterIP
				} else {
					src, dst = srcIPv6, config.RouterIP6
				}
				d.tunInbound <- &DataElem{
					data:   data,
					length: length,
					src:    src,
					dst:    dst,
				}
			}
			time.Sleep(time.Second)
		}
	}
}

func genICMPPacket(src net.IP, dst net.IP) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()
	icmpLayer := layers.ICMPv4{
		TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0),
		Id:       3842,
		Seq:      1,
	}
	ipLayer := layers.IPv4{
		Version:  4,
		SrcIP:    src,
		DstIP:    dst,
		Protocol: layers.IPProtocolICMPv4,
		Flags:    layers.IPv4DontFragment,
		TTL:      64,
		IHL:      5,
		Id:       55664,
	}
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}
	err := gopacket.SerializeLayers(buf, opts, &ipLayer, &icmpLayer)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize icmp packet, err: %v", err)
	}
	return buf.Bytes(), nil
}

func genICMPPacketIPv6(src net.IP, dst net.IP) ([]byte, error) {
	buf := gopacket.NewSerializeBuffer()
	icmpLayer := layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeEchoRequest, 0),
	}
	ipLayer := layers.IPv6{
		Version:    6,
		SrcIP:      src,
		DstIP:      dst,
		NextHeader: layers.IPProtocolICMPv6,
		HopLimit:   255,
	}
	opts := gopacket.SerializeOptions{
		FixLengths: true,
	}
	err := gopacket.SerializeLayers(buf, opts, &ipLayer, &icmpLayer)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize icmp6 packet, err: %v", err)
	}
	return buf.Bytes(), nil
}

func (d *Device) Start() {
	go d.readFromTun()
	for i := 0; i < d.thread; i++ {
		go d.parseIPHeader()
	}
	go d.writeToTun()
	go d.heartbeats()
}

func (h *tunHandler) HandleServer(ctx context.Context, tunConn net.Conn) {
	go h.printRoute()
	tun := &Device{
		tun:           tunConn,
		thread:        MaxThread,
		closed:        atomic.Bool{},
		tunInboundRaw: make(chan *DataElem, MaxSize),
		tunInbound:    make(chan *DataElem, MaxSize),
		tunOutbound:   make(chan *DataElem, MaxSize),
		chExit:        h.chExit,
	}
	defer tun.Close()
	tun.Start()

	for {
		select {
		case <-h.chExit:
			return
		case <-ctx.Done():
			return
		default:
		}
		func() {
			cancel, cancelFunc := context.WithCancel(ctx)
			defer cancelFunc()
			var lc net.ListenConfig
			packetConn, err := lc.ListenPacket(cancel, "udp", h.node.Addr)
			if err != nil {
				log.Debugf("[udp] can not listen %s, err: %v", h.node.Addr, err)
				return
			}
			err = h.transportTun(cancel, tun, packetConn)
			if err != nil {
				log.Debugf("[tun] %s: %v", tunConn.LocalAddr(), err)
			}
		}()
	}
}

type DataElem struct {
	data   []byte
	length int
	src    net.IP
	dst    net.IP
}

type udpElem struct {
	from   net.Addr
	data   []byte
	length int
	src    net.IP
	dst    net.IP
}

type Peer struct {
	conn   net.PacketConn
	thread int
	closed *atomic.Bool

	connInbound    chan *udpElem
	parsedConnInfo chan *udpElem

	tun    *Device
	routes *NAT

	errChan chan error
}

func (p *Peer) sendErr(err error) {
	select {
	case p.errChan <- err:
	default:
	}
}

func (p *Peer) readFromConn() {
	for {
		b := config.LPool.Get().([]byte)
		n, srcAddr, err := p.conn.ReadFrom(b[:])
		if err != nil {
			p.sendErr(err)
			return
		}
		if p.closed.Load() {
			return
		}
		p.connInbound <- &udpElem{
			from:   srcAddr,
			data:   b[:],
			length: n,
		}
	}
}

func (p *Peer) parseHeader() {
	for e := range p.connInbound {
		if util.IsIPv4(e.data[:e.length]) {
			// ipv4.ParseHeader
			b := e.data[:e.length]
			e.src = net.IPv4(b[12], b[13], b[14], b[15])
			e.dst = net.IPv4(b[16], b[17], b[18], b[19])
		} else if util.IsIPv6(e.data[:e.length]) {
			// ipv6.ParseHeader
			e.src = e.data[:e.length][8:24]
			e.dst = e.data[:e.length][24:40]
		} else {
			log.Errorf("[tun] unknown packet")
			continue
		}

		if _, loaded := p.routes.LoadOrStore(e.src, e.from); loaded {
			log.Debugf("[tun] add route: %s -> %s", e.src, e.from)
		} else {
			log.Debugf("[tun] new route: %s -> %s", e.src, e.from)
		}
		if p.closed.Load() {
			return
		}
		p.parsedConnInfo <- e
	}
}

func (p *Peer) route() {
	for e := range p.parsedConnInfo {
		if routeToAddr := p.routes.RouteTo(e.dst); routeToAddr != nil {
			log.Debugf("[tun] find route: %s -> %s", e.dst, routeToAddr)
			_, err := p.conn.WriteTo(e.data[:e.length], routeToAddr)
			config.LPool.Put(e.data[:])
			if err != nil {
				p.sendErr(err)
				return
			}
		} else {
			if !p.tun.closed.Load() {
				p.tun.tunOutbound <- &DataElem{
					data:   e.data,
					length: e.length,
					src:    e.src,
					dst:    e.dst,
				}
			}
		}
	}
}

func (p *Peer) Start() {
	go p.readFromConn()
	for i := 0; i < p.thread; i++ {
		go p.parseHeader()
	}
	go p.route()
}

func (p *Peer) Close() {
	p.closed.Store(true)
	p.conn.Close()
	close(p.connInbound)
	close(p.parsedConnInfo)
}

func (h *tunHandler) transportTun(ctx context.Context, tun *Device, conn net.PacketConn) error {
	errChan := make(chan error, 2)
	p := Peer{
		conn:           conn,
		thread:         MaxThread,
		closed:         &atomic.Bool{},
		connInbound:    make(chan *udpElem, MaxSize),
		parsedConnInfo: make(chan *udpElem, MaxSize),
		tun:            tun,
		routes:         h.routes,
		errChan:        errChan,
	}

	defer p.Close()
	p.Start()

	go func() {
		for e := range tun.tunInbound {
			select {
			case <-ctx.Done():
				return
			default:
			}

			addr := h.routes.RouteTo(e.dst)
			if addr == nil {
				config.LPool.Put(e.data[:])
				log.Debug(fmt.Errorf("[tun] no route for %s -> %s", e.src, e.dst))
				continue
			}

			log.Debugf("[tun] find route: %s -> %s", e.dst, addr)
			_, err := conn.WriteTo(e.data[:e.length], addr)
			config.LPool.Put(e.data[:])
			if err != nil {
				log.Debugf("[tun] can not route: %s -> %s", e.dst, addr)
				errChan <- err
				return
			}
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return nil
	}
}
