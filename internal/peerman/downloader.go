package peerman

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/Jamescog/bttclient/internal/data"
)

func RequestPiece(conn net.Conn, pieceIndex uint32, pieceLength uint64, peerIP string) error {
	piece := data.GetOrCreatePieceState(pieceIndex, pieceLength)

	data.MarkPieceAsRequested(pieceIndex)

	log.Printf("[Piece %d] Starting download from %s (%d blocks)", pieceIndex, peerIP, piece.TotalBlocks)

	return RequestNextBlocks(conn, pieceIndex, 5)
}

func RequestNextBlocks(conn net.Conn, pieceIndex uint32, count int) error {
	piece, exists := data.GetPieceState(pieceIndex)
	if !exists {
		return fmt.Errorf("piece %d not found", pieceIndex)
	}

	requested := 0
	for blockIndex := uint32(0); blockIndex < piece.TotalBlocks && requested < count; blockIndex++ {
		if data.IsBlockRequested(pieceIndex, blockIndex) {
			continue
		}

		offset, length := CalculateBlockInfo(pieceIndex, blockIndex, piece.TotalLength, piece.BlockSize)

		if err := sendBlockRequest(conn, pieceIndex, offset, length); err != nil {
			return err
		}

		data.MarkBlockRequested(pieceIndex, blockIndex)
		requested++
	}

	return nil
}

func CalculateBlockInfo(pieceIndex, blockIndex uint32, pieceLength uint64, blockSize uint32) (uint32, uint32) {
	offset := blockIndex * blockSize
	remaining := pieceLength - uint64(offset)

	length := blockSize
	if uint64(length) > remaining {
		length = uint32(remaining)
	}

	return offset, length
}

func sendBlockRequest(conn net.Conn, pieceIndex, begin, length uint32) error {
	req := make([]byte, 17)
	binary.BigEndian.PutUint32(req[0:4], 13)
	req[4] = 6
	binary.BigEndian.PutUint32(req[5:9], pieceIndex)
	binary.BigEndian.PutUint32(req[9:13], begin)
	binary.BigEndian.PutUint32(req[13:17], length)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	return nil
}

func HandleBlockReceived(pieceIndex, offset uint32, blockData []byte, conn net.Conn, peerIP string) (bool, error) {
	piece, exists := data.GetPieceState(pieceIndex)
	if !exists {
		return false, fmt.Errorf("piece %d not found", pieceIndex)
	}

	blockIndex := offset / piece.BlockSize

	if data.IsBlockReceived(pieceIndex, blockIndex) {
		return false, nil
	}

	piece.Mu.Lock()
	copy(piece.Buffer[offset:], blockData)
	piece.Mu.Unlock()

	data.MarkBlockReceived(pieceIndex, blockIndex)

	if data.IsPieceFullyReceived(pieceIndex) {
		if err := VerifyAndSavePiece(pieceIndex); err != nil {
			log.Printf("[Piece %d] Verification failed: %v - will retry", pieceIndex, err)
			data.ResetPieceForRetry(pieceIndex)
			return false, nil
		}

		log.Printf("[Piece %d] Download complete and verified from %s", pieceIndex, peerIP)
		return true, nil
	}

	if err := RequestNextBlocks(conn, pieceIndex, 1); err != nil {
		return false, err
	}

	return false, nil
}
