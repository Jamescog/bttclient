package peerman

import (
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Jamescog/bttclient/internal/data"
)

var (
	pieceHashes []byte
	outputFile  *os.File
	fileName    string
)

func InitializeDownload(torrentName string, pieceHashesData []byte, totalSize int64) error {
	pieceHashes = pieceHashesData
	fileName = torrentName

	data.TotalFileSize = totalSize

	var err error
	outputFile, err = os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}

	if err := outputFile.Truncate(totalSize); err != nil {
		outputFile.Close()
		return fmt.Errorf("failed to allocate file space: %w", err)
	}

	log.Printf("Initialized download: %s (%.2f MB)", fileName, float64(totalSize)/(1024*1024))
	return nil
}

func VerifyAndSavePiece(pieceIndex uint32) error {
	piece, exists := data.GetPieceState(pieceIndex)
	if !exists {
		return fmt.Errorf("piece %d not found", pieceIndex)
	}

	piece.Mu.Lock()
	buffer := piece.Buffer
	pieceLength := piece.TotalLength
	piece.Mu.Unlock()

	if len(pieceHashes) == 0 {
		return fmt.Errorf("piece hashes not initialized")
	}

	expectedHash := pieceHashes[pieceIndex*20 : (pieceIndex+1)*20]

	actualHash := sha1.Sum(buffer[:pieceLength])

	if string(actualHash[:]) != string(expectedHash) {
		piece.Mu.Lock()
		piece.IsVerified = false
		piece.IsComplete = false
		piece.Mu.Unlock()
		return fmt.Errorf("hash mismatch for piece %d", pieceIndex)
	}

	offset := int64(pieceIndex) * int64(pieceLength)
	if _, err := outputFile.WriteAt(buffer[:pieceLength], offset); err != nil {
		return fmt.Errorf("failed to write piece %d to disk: %w", pieceIndex, err)
	}

	piece.Mu.Lock()
	piece.IsVerified = true
	piece.IsSaved = true
	piece.Mu.Unlock()

	data.AddDownloadedBytes(int64(pieceLength))

	return nil
}

func CloseDownload() error {
	if outputFile != nil {
		if err := outputFile.Sync(); err != nil {
			return err
		}
		return outputFile.Close()
	}
	return nil
}

func GetOutputPath() string {
	if outputFile != nil {
		absPath, _ := filepath.Abs(fileName)
		return absPath
	}
	return ""
}
