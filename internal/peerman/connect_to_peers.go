package peerman

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
)

type Peer struct {
	IP   string
	Port int
}

// Build handshake message
func buildHandshake(infoHash, peerID []byte) []byte {
	pstr := "BitTorrent protocol"
	buf := make([]byte, 1+len(pstr)+8+20+20) //<pstrlen><pstr><reserved><info_hash><peer_id>
	buf[0] = byte(len(pstr))

	copy(buf[1:], []byte(pstr))

	copy(buf[1+19+8:], infoHash)

	copy(buf[1+19+8+20:], peerID)
	return buf
}

func ConnectToPeer(ctx context.Context, peer Peer, infoHash, peerID [20]byte) error {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", peer.IP, peer.Port))

	if err != nil {
		return fmt.Errorf("dial faild: %w", err)
	}

	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	handshake := buildHandshake(infoHash[:], peerID[:])

	if _, err := conn.Write(handshake); err != nil {
		return fmt.Errorf("write handshake: %w", err)

	}

	resp := make([]byte, 68)

	if _, err := conn.Read(resp); err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}

	peerInfoHash := resp[28:48]

	if string(peerInfoHash) != string(infoHash[:]) {
		return fmt.Errorf("info hash mismatch. got %s: expected: %s", string(peerInfoHash), string(infoHash[:]))
	}

	log.Printf("Successfully connected to peer %s:%d", peer.IP, peer.Port)
	return nil

}
