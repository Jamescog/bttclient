package udp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
	"net"
	"time"
)

const (
	protocolID     uint64 = 0x41727101980 // UDP magic constant per BEP-15
	actionConnect  uint32 = 0
	actionAnnounce uint32 = 1
	actionScrape   uint32 = 2
	actionError    uint32 = 3
)

//function to generate transaction id

func GenerateTransactionId() uint32 {
	return mathrand.Uint32()
}

func SendConnect(conn *net.UDPConn) (uint64, uint32, error) {
	var buf [16]byte

	binary.BigEndian.PutUint64(buf[0:8], protocolID)
	binary.BigEndian.PutUint32(buf[8:12], uint32(actionConnect))
	tx := GenerateTransactionId()
	binary.BigEndian.PutUint32(buf[12:16], tx)

	if _, err := conn.Write(buf[:]); err != nil {
		return 0, 0, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	resp := make([]byte, 32)

	n, err := conn.Read(resp)

	if err != nil {
		return 0, 0, err
	}

	if n < 16 {
		return 0, 0, errors.New("connect response too short")
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	respTx := binary.BigEndian.Uint32(resp[4:8])

	if action == actionError {
		return 0, 0, fmt.Errorf("tracker error: %s", resp[8:n])
	}

	if action != actionConnect {
		return 0, 0, fmt.Errorf("unexpected connect response (action=%d tx=%d)", action, respTx)
	}

	connectionID := binary.BigEndian.Uint64(resp[8:16])

	return connectionID, tx, nil

}

func SendAnnounce(conn *net.UDPConn, connectionID uint64, infoHash [20]byte, peerID [20]byte, port uint16, downloaded uint64, left uint64, uploaded uint64) ([]net.TCPAddr, error) {

	buf := bytes.NewBuffer(make([]byte, 0, 98))

	_ = binary.Write(buf, binary.BigEndian, connectionID)
	_ = binary.Write(buf, binary.BigEndian, uint32(actionAnnounce))
	tx := GenerateTransactionId()

	_ = binary.Write(buf, binary.BigEndian, tx)

	_, _ = buf.Write(infoHash[:])
	_, _ = buf.Write(peerID[:])

	_ = binary.Write(buf, binary.BigEndian, downloaded)

	_ = binary.Write(buf, binary.BigEndian, left)
	_ = binary.Write(buf, binary.BigEndian, uploaded)
	// event (4) 0 = none, 1 = completed, 2 = started, 3 = stopped
	_ = binary.Write(buf, binary.BigEndian, uint32(2)) // e.g., "started"
	// IP address (4) - 0 default (tracker uses src IP usually)
	_ = binary.Write(buf, binary.BigEndian, uint32(0))
	// key (4) - random
	_ = binary.Write(buf, binary.BigEndian, GenerateTransactionId())
	// num_want (4) - -1 default (0xFFFFFFFF) means default
	_ = binary.Write(buf, binary.BigEndian, int32(-1))
	// port (2)
	_ = binary.Write(buf, binary.BigEndian, port)

	if _, err := conn.Write(buf.Bytes()); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	resp := make([]byte, 1500)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	if n < 20 {
		return nil, errors.New("announce response too short")
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	respTx := binary.BigEndian.Uint32(resp[4:8])
	if action == actionError {
		return nil, fmt.Errorf("tracker error: %s", string(resp[8:n]))
	}
	if action != actionAnnounce || respTx != tx {
		return nil, fmt.Errorf("unexpected announce response (action=%d tx=%d)", action, respTx)
	}

	interval := binary.BigEndian.Uint32(resp[8:12])
	leechers := binary.BigEndian.Uint32(resp[12:16])
	seeders := binary.BigEndian.Uint32(resp[16:20])
	_ = interval // we could use it
	_ = leechers
	_ = seeders

	peersData := resp[20:n]
	peers := []net.TCPAddr{}
	for i := 0; i+6 <= len(peersData); i += 6 {
		ip := net.IPv4(peersData[i], peersData[i+1], peersData[i+2], peersData[i+3])
		port := binary.BigEndian.Uint16(peersData[i+4 : i+6])
		peers = append(peers, net.TCPAddr{IP: ip, Port: int(port)})
	}

	log.Printf("announce: interval=%d seeders=%d leechers=%d peers=%d\n", interval, seeders, leechers, len(peers))
	return peers, nil
}
