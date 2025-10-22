package bencode

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/gofrs/uuid/v5"
)

// findInfoSection locates the raw bencoded bytes of the top-level "info" dictionary
// in a .torrent file. It returns the start (inclusive) and end (exclusive) indices
// of the info dictionary value within data.
func findInfoSection(data []byte) (int, int, error) {
	if len(data) == 0 || data[0] != 'd' {
		return 0, 0, fmt.Errorf("torrent must start with a bencoded dictionary")
	}

	// Scan top-level dictionary: d <key><value> ... e
	i := 1
	for i < len(data) && data[i] != 'e' {
		// Decode key (must be a byte string)
		keyRaw, next, err := DecodeNext(data, i)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to decode top-level key at %d: %w", i, err)
		}
		keyBytes, ok := keyRaw.([]byte)
		if !ok {
			return 0, 0, fmt.Errorf("top-level dictionary key is not a string at %d", i)
		}
		key := string(keyBytes)
		i = next

		// Value starts at i
		if key == "info" {
			start := i
			_, end, err := DecodeNext(data, start)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to decode info dictionary: %w", err)
			}
			return start, end, nil
		}

		// Skip value for non-matching keys
		_, nextVal, err := DecodeNext(data, i)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to skip value for key %q: %w", key, err)
		}
		i = nextVal
	}

	return 0, 0, fmt.Errorf("info dictionary not found in torrent")
}

// InfoHash computes the SHA-1 of the raw bencoded info dictionary bytes.
// It returns the 20-byte digest.
func InfoHash(data []byte) ([20]byte, error) {
	var zero [20]byte
	start, end, err := findInfoSection(data)
	if err != nil {
		return zero, err
	}
	sum := sha1.Sum(data[start:end])
	return sum, nil
}

// InfoHashHex computes the info hash and returns it as a hex string.
func InfoHashHex(data []byte) (string, error) {
	sum, err := InfoHash(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum[:]), nil
}

// InfoHashHexFromFile is a convenience to compute the info hash hex string directly from a file path.
func InfoHashHexFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return InfoHashHex(data)
}

func RandomPeerID() (string, error) {
	u, err := uuid.NewV4()
	if err != nil {
		return "", fmt.Errorf("error generating UUID for peer ID: %w", err)
	}
	return "-BTTC01-" + u.String()[:12], nil
}

func GenerateTrackerURL(torrentFilePath string, uploaded int64, downloaded int64, left int64) (string, error) {
	torrent, err := DecodeTorrentFile(torrentFilePath)
	if err != nil {
		return "", fmt.Errorf("error decoding torrent file: %w", err)
	}

	// Compute raw 20-byte info hash (SHA-1 of bencoded info dict)
	data, err := os.ReadFile(torrentFilePath)
	if err != nil {
		return "", fmt.Errorf("error reading torrent file: %w", err)
	}
	sum, err := InfoHash(data)
	if err != nil {
		return "", fmt.Errorf("error computing info hash: %w", err)
	}

	peerId, err := RandomPeerID()
	if err != nil {
		return "", fmt.Errorf("error generating peer ID: %w", err)
	}
	port := 6881

	// Per BitTorrent spec, info_hash and peer_id must be percent-encoded as raw bytes
	infoHashParam := strictPercentEncode(sum[:])
	peerIDParam := strictPercentEncode([]byte(peerId))

	trackerURL := fmt.Sprintf("%s?info_hash=%s&peer_id=%s&port=%d&uploaded=%d&downloaded=%d&left=%d&compact=1",
		torrent.Announce(), infoHashParam, peerIDParam, port, uploaded, downloaded, left)

	return trackerURL, nil

}

// strictPercentEncode percent-encodes each byte as %XX (uppercase hex), suitable for
// BitTorrent tracker query parameters like info_hash and peer_id.
func strictPercentEncode(b []byte) string {
	// Each byte becomes 3 chars: %XX
	out := make([]byte, 0, len(b)*3)
	const hexdigits = "0123456789ABCDEF"
	for _, c := range b {
		out = append(out, '%')
		out = append(out, hexdigits[c>>4])
		out = append(out, hexdigits[c&0x0F])
	}
	return string(out)
}
