package core

import (
	"context"
	"errors"
	"fmt"
	"github.com/wencaiwulue/kubevpn/util"
	"net"
)

var (
	// ErrEmptyChain is an error that implies the chain is empty.
	ErrEmptyChain = errors.New("empty chain")
)

// Chain is a proxy chain that holds a list of proxy node groups.
type Chain struct {
	isRoute bool
	Retries int
	node    *Node
}

// NewChain creates a proxy chain with a list of proxy nodes.
// It creates the node groups automatically, one group per node.
func NewChain() *Chain {
	return &Chain{}
}

// newRoute creates a chain route.
// a chain route is the final route after node selection.
func newRoute() *Chain {
	chain := NewChain()
	chain.isRoute = true
	return chain
}

// Nodes returns the proxy nodes that the chain holds.
// The first node in each group will be returned.
func (c *Chain) Nodes() *Node {
	return c.node
}

// LastNode returns the last node of the node list.
// If the chain is empty, an empty node will be returned.
// If the last node is a node group, the first node in the group will be returned.
func (c *Chain) LastNode() *Node {
	if c.IsEmpty() {
		return &Node{}
	}
	return c.node
}

// AddNode appends the node(s) to the chain.
func (c *Chain) AddNode(node *Node) {
	if c == nil {
		return
	}
	c.node = node
}

// AddNodeGroup appends the group(s) to the chain.
func (c *Chain) AddNodeGroup(groups *Node) {
	if c == nil {
		return
	}
	c.node = groups
}

// IsEmpty checks if the chain is empty.
// An empty chain means that there is no proxy node or node group in the chain.
func (c *Chain) IsEmpty() bool {
	return c == nil || c.node == nil
}

// DialContext connects to the address on the named network using the provided context.
func (c *Chain) DialContext(ctx context.Context, network, address string) (conn net.Conn, err error) {
	retries := 1
	if c != nil && c.Retries > 0 {
		retries = c.Retries
	}

	for i := 0; i < retries; i++ {
		conn, err = c.dial(ctx, network, address)
		if err == nil {
			break
		}
	}
	return
}

func (c *Chain) dial(ctx context.Context, network, address string) (net.Conn, error) {
	route, err := c.selectRouteFor(address)
	if err != nil {
		return nil, err
	}

	ipAddr := address
	if address != "" {
		ipAddr = c.resolve(address)
	}

	if route.IsEmpty() {
		switch network {
		case "udp", "udp4", "udp6":
			if address == "" {
				return net.ListenUDP(network, nil)
			}
		default:
		}
		d := &net.Dialer{
			Timeout: util.DialTimeout,
			// LocalAddr: laddr, // TODO: optional local address
		}
		return d.DialContext(ctx, network, ipAddr)
	}

	conn, err := route.getConn(ctx)
	if err != nil {
		return nil, err
	}

	cc, err := route.LastNode().Client.ConnectContext(ctx, conn, network, ipAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return cc, nil
}

func (*Chain) resolve(addr string) string {
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if ips, err := net.LookupIP(host); err == nil && len(ips) > 0 {
			return fmt.Sprintf("%s:%s", ips[0].String(), port)
		}
	}
	return addr
}

// Conn obtains a handshaked connection to the last node of the chain.
func (c *Chain) Conn() (conn net.Conn, err error) {
	ctx := context.Background()

	retries := 1
	if c != nil && c.Retries > 0 {
		retries = c.Retries
	}

	for i := 0; i < retries; i++ {
		var route *Chain
		route, err = c.selectRoute()
		if err != nil {
			continue
		}
		conn, err = route.getConn(ctx)
		if err == nil {
			break
		}
	}
	return
}

// getConn obtains a connection to the last node of the chain.
func (c *Chain) getConn(_ context.Context) (conn net.Conn, err error) {
	if c.IsEmpty() {
		err = ErrEmptyChain
		return
	}
	node := c.Nodes()
	cc, err := node.Client.Dial(node.Addr)
	if err != nil {
		return
	}

	conn = cc
	return
}

func (c *Chain) selectRoute() (route *Chain, err error) {
	return c.selectRouteFor("")
}

// selectRouteFor selects route with bypass testing.
func (c *Chain) selectRouteFor(addr string) (route *Chain, err error) {
	if c.IsEmpty() {
		return newRoute(), nil
	}
	if c.isRoute {
		return c, nil
	}

	route = newRoute()
	route.AddNode(c.node)
	return
}