package main

import (
	"fmt"

	"github.com/Jamescog/bttclient/pkg/bencode"
)

func main() {
	filename := "big-buck-bunny.torrent"

	torrent, err := bencode.DecodeTorrentFile(filename)
	if err != nil {
		fmt.Println("Error decoding torrent:", err)
		return
	}

	// Access specific fields
	fmt.Printf("Announce: %s\n", torrent.Announce())
	fmt.Printf("Name: %s\n", torrent.Name())
	fmt.Printf("Piece Length: %d\n", torrent.PieceLength())
}
