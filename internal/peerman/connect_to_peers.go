package peerman

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/Jamescog/bttclient/internal/data"
	"github.com/Jamescog/bttclient/pkg/protocol"
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

func ConnectToPeer(ctx context.Context, peer Peer, infoHash, peerID [20]byte) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}

	conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", peer.IP, peer.Port))

	if err != nil {
		return nil, fmt.Errorf("dial faild: %w", err)
	}

	// defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	handshake := buildHandshake(infoHash[:], peerID[:])

	if _, err := conn.Write(handshake); err != nil {
		return nil, fmt.Errorf("write handshake: %w", err)

	}

	resp := make([]byte, 68)

	if _, err := conn.Read(resp); err != nil {
		return nil, fmt.Errorf("read handshake: %w", err)
	}

	peerInfoHash := resp[28:48]

	if string(peerInfoHash) != string(infoHash[:]) {
		return nil, fmt.Errorf("info hash mismatch. got %s: expected: %s", string(peerInfoHash), string(infoHash[:]))
	}

	log.Printf("Successfully connected to peer %s:%d", peer.IP, peer.Port)
	return conn, nil

}

func readMessage(conn net.Conn) ([]byte, error) {
	lengthBuf := make([]byte, 4)

	if _, err := io.ReadFull(conn, lengthBuf); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	if length == 0 {
		return lengthBuf, nil
	}

	msg := make([]byte, length)
	if _, err := io.ReadFull(conn, msg); err != nil {
		return nil, err
	}
	return append(lengthBuf, msg...), nil
}

func HandlePeer(peer Peer, conn net.Conn, pieceLength int) {
	defer conn.Close()

	if _, err := conn.Write([]byte{0, 0, 0, 1, 2}); err != nil {
		log.Printf("Failed to send 'interested' to %s: %v", peer.IP, err)
		return
	}

	for {
		conn.SetDeadline(time.Now().Add(120 * time.Second))
		msg, err := readMessage(conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("Peer %s timeout", peer.IP)
				data.RemoveClient(peer.IP)
			} else {
				log.Printf("Peer %s disconnected", peer.IP)
				data.RemoveClient(peer.IP)
			}

			return
		}
		if len(msg) < 4 {
			continue
		}
		if len(msg) == 4 {
			continue
		}

		msgID := msg[4]

		switch msgID {
		case 0:
			data.ChokeClient(peer.IP)
		case 1:
			data.UnchokeClient(peer.IP)

			pieceIndex := data.SelectNextPiece(peer.IP)
			if pieceIndex >= 0 {
				if err := RequestPiece(conn, uint32(pieceIndex), uint64(pieceLength), peer.IP); err != nil {
					log.Printf("Failed to request piece %d: %v", pieceIndex, err)
				}
			}
		case 2:
		case 3:
		case 4:
			if len(msg) >= 9 {
				pieceIndex := binary.BigEndian.Uint32(msg[5:9])
				data.AddHavePiece(peer.IP, peer.Port, pieceIndex)
			}
		case 5:
			payload := msg[5:]
			bitfield := protocol.PiecesPeerHas(payload)
			data.AddPiecesForClient(peer.IP, peer.Port, bitfield)
		case 7:
			if len(msg) >= 13 {
				index := binary.BigEndian.Uint32(msg[5:9])
				begin := binary.BigEndian.Uint32(msg[9:13])
				blockData := msg[13:]

				pieceComplete, err := HandleBlockReceived(index, begin, blockData, conn, peer.IP)
				if err != nil {
					log.Printf("Error handling block: %v", err)
					continue
				}

				if pieceComplete {
					nextPieceIndex := data.SelectNextPiece(peer.IP)
					if nextPieceIndex >= 0 {
						if err := RequestPiece(conn, uint32(nextPieceIndex), uint64(pieceLength), peer.IP); err != nil {
							log.Printf("Failed to request next piece: %v", err)
						}
					}
				}
			}
		case 0xFF:
		default:
		}
	}

}
