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
	"sync"
	"time"

	"github.com/Jamescog/bttclient/internal/data"
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
	fmt.Printf("Total Length: %d bytes\n", torrent.Length())
	fmt.Printf("Number of pieces: %d\n", torrent.NumPieces())
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

	if err := peerman.InitializeDownload(torrent.Name(), torrent.Pieces(), torrent.Length()); err != nil {
		log.Fatalf("failed to initialize download: %v", err)
	}
	defer peerman.CloseDownload()

	var wg sync.WaitGroup
	attempted := 0

	sem := make(chan struct{}, 57)

	for _, peer := range peers {

		if attempted > 57 {
			break
		}
		attempted++

		p := peerman.Peer{IP: peer.IP.String(), Port: peer.Port}
		wg.Add(1)
		go func(p peerman.Peer) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()

			conn, err := peerman.ConnectToPeer(ctx, p, infoHash, peerID)
			if err != nil {
				log.Printf("Failed to handshake with %s:%d: %v", p.IP, p.Port, err)
				return
			}
			peerman.HandlePeer(p, conn, torrent.PieceLength())
		}(p)

	}
	go printPeriodicStats()
	wg.Wait()

	log.Printf("Download complete! File saved to: %s", peerman.GetOutputPath())
}

func printPeriodicStats() {
	for {
		time.Sleep(15 * time.Second)
		data.PrintClientStates()
	}
}
