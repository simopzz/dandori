package apptest

import (
	"io"
	"net"
	"sync"
	"testing"
)

// proxy forwards TCP connections to the database and can be cut to make the
// database unreachable without touching the server itself.
type proxy struct {
	ln     net.Listener
	target string

	mu    sync.Mutex
	up    bool
	conns map[net.Conn]struct{}
}

func startProxy(t testing.TB, target string) *proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &proxy{ln: ln, target: target, up: true, conns: map[net.Conn]struct{}{}}
	go p.serve()
	t.Cleanup(func() {
		ln.Close()
		p.setUp(false)
	})
	return p
}

func (p *proxy) addr() string { return p.ln.Addr().String() }

func (p *proxy) setUp(up bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.up = up
	if !up {
		for c := range p.conns {
			c.Close()
		}
		clear(p.conns)
	}
}

func (p *proxy) serve() {
	for {
		client, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.forward(client)
	}
}

func (p *proxy) forward(client net.Conn) {
	p.mu.Lock()
	up := p.up
	p.mu.Unlock()
	if !up {
		client.Close()
		return
	}
	server, err := net.Dial("tcp", p.target)
	if err != nil {
		client.Close()
		return
	}
	p.mu.Lock()
	if !p.up {
		p.mu.Unlock()
		client.Close()
		server.Close()
		return
	}
	p.conns[client] = struct{}{}
	p.conns[server] = struct{}{}
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		io.Copy(dst, src)
		done <- struct{}{}
	}
	go pipe(server, client)
	go pipe(client, server)
	<-done
	client.Close()
	server.Close()
	p.mu.Lock()
	delete(p.conns, client)
	delete(p.conns, server)
	p.mu.Unlock()
}
