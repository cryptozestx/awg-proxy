package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

type SOCKS5Server struct {
	listener net.Listener
	dialer   ContextDialer
}

type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

func NewSOCKS5Server(port int, dialer ContextDialer) (*SOCKS5Server, int, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	actualPort := l.Addr().(*net.TCPAddr).Port
	return &SOCKS5Server{
		listener: l,
		dialer:   dialer,
	}, actualPort, nil
}

func (s *SOCKS5Server) Start() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // Listener closed
		}
		go s.handleConnection(conn)
	}
}

func (s *SOCKS5Server) Close() error {
	return s.listener.Close()
}

func (s *SOCKS5Server) handleConnection(client net.Conn) {
	defer client.Close()

	// 1. Negotiation Phase
	var buf [257]byte
	// Read version and number of methods
	if _, err := io.ReadFull(client, buf[:2]); err != nil {
		return
	}

	if buf[0] != 0x05 {
		return // SOCKS version must be 5
	}

	numMethods := int(buf[1])
	if _, err := io.ReadFull(client, buf[:numMethods]); err != nil {
		return
	}

	// Respond with 'No Authentication Required' (0x00)
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 2. Request Phase
	// Read header: VER(1), CMD(1), RSV(1), ATYP(1)
	if _, err := io.ReadFull(client, buf[:4]); err != nil {
		return
	}

	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != 0x05 {
		return
	}

	if cmd != 0x01 { // Only CONNECT command is supported
		s.sendReply(client, 0x07, nil, 0) // Command not supported
		return
	}

	var destAddr string

	switch atyp {
	case 0x01: // IPv4
		var ipBuf [4]byte
		if _, err := io.ReadFull(client, ipBuf[:]); err != nil {
			return
		}
		destAddr = net.IP(ipBuf[:]).String()
	case 0x03: // Domain name
		var lenBuf [1]byte
		if _, err := io.ReadFull(client, lenBuf[:]); err != nil {
			return
		}
		domainLen := int(lenBuf[0])
		domainBuf := make([]byte, domainLen)
		if _, err := io.ReadFull(client, domainBuf); err != nil {
			return
		}
		destAddr = string(domainBuf)
	case 0x04: // IPv6
		var ipBuf [16]byte
		if _, err := io.ReadFull(client, ipBuf[:]); err != nil {
			return
		}
		destAddr = fmt.Sprintf("[%s]", net.IP(ipBuf[:]).String())
	default:
		s.sendReply(client, 0x08, nil, 0) // Address type not supported
		return
	}

	// Read port (2 bytes)
	var portBuf [2]byte
	if _, err := io.ReadFull(client, portBuf[:]); err != nil {
		return
	}
	destPort := binary.BigEndian.Uint16(portBuf[:])

	targetAddr := net.JoinHostPort(destAddr, strconv.Itoa(int(destPort)))

	// 3. Dial Remote Host through Netstack
	remote, err := s.dialer.DialContext(context.Background(), "tcp", targetAddr)
	if err != nil {
		log.Printf("[SOCKS5] Failed to dial %s: %v", targetAddr, err)
		s.sendReply(client, 0x04, nil, 0) // Host unreachable
		return
	}
	defer remote.Close()

	// Get local address to send in reply
	localAddr := remote.LocalAddr().(*net.TCPAddr)
	s.sendReply(client, 0x00, localAddr.IP, uint16(localAddr.Port))

	// 4. Relay Traffic bidirectionally
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(client, remote)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(remote, client)
		errChan <- err
	}()

	<-errChan
}

func (s *SOCKS5Server) sendReply(client net.Conn, rep byte, bindIP net.IP, bindPort uint16) {
	var atyp byte = 0x01
	ipBytes := make([]byte, 4)
	if bindIP != nil {
		if ip4 := bindIP.To4(); ip4 != nil {
			copy(ipBytes, ip4)
		} else if ip6 := bindIP.To16(); ip6 != nil {
			atyp = 0x04
			ipBytes = ip6
		}
	}

	reply := make([]byte, 0, 4+len(ipBytes)+2)
	reply = append(reply, 0x05, rep, 0x00, atyp)
	reply = append(reply, ipBytes...)

	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, bindPort)
	reply = append(reply, portBytes...)

	_, _ = client.Write(reply)
}

// Helpers to parse netip addresses
func ParseAddresses(addrs []string) ([]netip.Addr, error) {
	var result []netip.Addr
	for _, a := range addrs {
		// Strip CIDR mask if present
		ipStr := a
		if idx := strings.Index(a, "/"); idx >= 0 {
			ipStr = a[:idx]
		}
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address: %s", a)
		}
		result = append(result, ip)
	}
	return result, nil
}
