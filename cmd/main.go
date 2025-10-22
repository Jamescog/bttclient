package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"time"

	tracker "github.com/Jamescog/bttclient/internal/trackers/udp"

	"github.com/Jamescog/bttclient/internal/peerman"
	"github.com/Jamescog/bttclient/pkg/bencode"
)

func main() {

	filename := flag.String("file", "", "Path to input file (required)")
	_ = flag.Bool("v", false, "Enable verbose mode (optional)")

	// Parse flags

	flag.Parse()

	if *filename == "" {
		fmt.Println("Error: -file flag is required")
		flag.Usage()
		return
	}

	torrent, err := bencode.DecodeTorrentFile(*filename)
	if err != nil {
		fmt.Println("Error decoding torrent:", err)
		return
	}

	// Compute info hash
	infoHashHex, err := bencode.InfoHashHexFromFile(*filename)
	if err != nil {
		fmt.Println("Error computing info hash:", err)
		return
	}

	trackerURL, err := bencode.GenerateTrackerURL(*filename, 0, 0, 0)
	if err != nil {
		fmt.Println("Error generating tracker URL:", err)
		return
	}
	fmt.Printf("Tracker URL: %s\n", trackerURL)

	fmt.Printf("Announce: %s\n", torrent.Announce())
	fmt.Printf("Name: %s\n", torrent.Name())
	fmt.Printf("Piece Length: %d\n", torrent.PieceLength())
	fmt.Printf("Info Hash: %s\n", infoHashHex)

	u, err := url.Parse(torrent.Announce())
	if err != nil {
		log.Fatalf("failed to unescape announce URL: %v", err)
	}

	trackerAddr := u.Host

	fmt.Printf("Contacting tracker at %s...\n", trackerAddr)

	udpAddr, err := net.ResolveUDPAddr("udp", trackerAddr)

	if err != nil {
		log.Fatalf("resolve: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}

	defer conn.Close()
	var connectionID uint64
	var tx uint32
	retries := 3

	for i := 0; i < retries; i++ {
		connectionID, tx, err = tracker.SendConnect(conn)
		if err == nil {
			break
		}
		log.Printf("connect attempt %d failed: %v", i+1, err)
		time.Sleep(time.Duration(math.Pow(2, float64(i))) * time.Second)
	}

	if err != nil {
		log.Fatalf("connect failed after retries: %v", err)
	}
	log.Printf("got connectionID=%d tx=%d\n", connectionID, tx)

	var infoHash [20]byte
	infoHashBytes, err := hex.DecodeString(infoHashHex)
	if err != nil {
		log.Fatalf("failed to decode info hash hex: %v", err)
	}
	copy(infoHash[:], infoHashBytes)

	var peerID [20]byte

	peerIDStr, err := bencode.RandomPeerID()
	if err != nil {
		log.Fatalf("failed to generate peer ID: %v", err)
	}
	copy(peerID[:], []byte(peerIDStr))

	_, err = rand.Read(peerID[:])
	if err != nil {
		log.Fatalf("failed to generate peer ID: %v", err)
	}

	peers, err := tracker.SendAnnounce(conn, connectionID, infoHash, peerID, 6881, 0, uint64(torrent.PieceLength()), 0)
	if err != nil {
		log.Fatalf("announce failed: %v", err)
	}

	log.Println("peers:")
	// for _, p := range peers {
	// 	fmt.Printf(" - %s:%d\n", p.IP.String(), p.Port)
	// }

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results := make(chan error, len(peers))

	for _, peer := range peers {
		p := peerman.Peer{peer.IP.String(), peer.Port}

		go func(p peerman.Peer) {
			err := peerman.ConnectToPeer(ctx, p, infoHash, peerID)
			results <- err
		}(p)

	}

	successCount := 0

	for i := 0; i < len(peers); i++ {
		select {
		case err := <-results:
			{
				if err == nil {
					successCount++
				} else {
					log.Printf("failed to connect: %v", err)
				}
			}
		case <-ctx.Done():
			log.Println("overall timeout reached")
		}
	}

	log.Printf("✅ Connected to %d out of %d peers", successCount, len(peers))

}
