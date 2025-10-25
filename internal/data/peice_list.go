package data

import (
	"log"
	"math/rand"
	"sync"
)

type PieceState struct {
	Mu              sync.Mutex
	PieceNo         uint32
	InfoHash        string
	TotalLength     uint64
	NextOffset      uint64
	IsComplete      bool
	IsVerified      bool
	IsSaved         bool
	ClientState     *ClientState
	Buffer          []byte
	IsRequested     bool
	BlockSize       uint32
	TotalBlocks     uint32
	ReceivedBlocks  map[uint32]bool
	RequestedBlocks map[uint32]bool
}

type ClientState struct {
	Mu     sync.Mutex
	IP     string
	Port   int
	Choked bool
	Pieces []uint32
}

var (
	GlobalClientList = make(map[string]*ClientState)
	globalMu         sync.RWMutex
	GlobalPieceList  = make(map[uint32]*PieceState)
	globalPieceMu    sync.RWMutex
	TotalFileSize    int64
	DownloadedBytes  int64
	downloadedMu     sync.Mutex
)

func AddPiecesForClient(ip string, port int, pieces []uint32) *ClientState {
	globalMu.Lock()
	defer globalMu.Unlock()

	if client, exists := GlobalClientList[ip]; exists {
		client.Mu.Lock()
		client.Pieces = make([]uint32, len(pieces))
		copy(client.Pieces, pieces)
		client.Mu.Unlock()
		return client
	}

	client := &ClientState{
		IP:     ip,
		Port:   port,
		Choked: true,
		Pieces: make([]uint32, len(pieces)),
	}
	copy(client.Pieces, pieces)
	GlobalClientList[ip] = client
	return client
}

func RemoveClient(ip string) {
	globalMu.Lock()
	defer globalMu.Unlock()
	delete(GlobalClientList, ip)
}

func ChokeClient(ip string) *ClientState {
	globalMu.RLock()
	client, exists := GlobalClientList[ip]
	globalMu.RUnlock()
	if !exists {
		return nil
	}

	client.Mu.Lock()
	defer client.Mu.Unlock()
	client.Choked = true
	return client
}

func UnchokeClient(ip string) *ClientState {
	globalMu.RLock()
	client, exists := GlobalClientList[ip]
	globalMu.RUnlock()
	if !exists {
		return nil
	}

	client.Mu.Lock()
	defer client.Mu.Unlock()
	client.Choked = false
	return client
}

// PrintClientStates logs the state of all clients in the global list.
func PrintClientStates() {
	globalMu.RLock()
	defer globalMu.RUnlock()

	totalPeers := len(GlobalClientList)
	unchokedCount := 0

	for _, client := range GlobalClientList {
		client.Mu.Lock()
		if !client.Choked {
			unchokedCount++
		}
		client.Mu.Unlock()
	}

	globalPieceMu.RLock()
	completedPieces := 0
	inProgressPieces := 0
	for _, piece := range GlobalPieceList {
		piece.Mu.Lock()
		if piece.IsComplete && piece.IsVerified {
			completedPieces++
		} else if piece.IsRequested {
			inProgressPieces++
		}
		piece.Mu.Unlock()
	}
	totalPieces := len(GlobalPieceList)
	globalPieceMu.RUnlock()

	var piecePercentage float64
	if totalPieces > 0 {
		piecePercentage = (float64(completedPieces) / float64(totalPieces)) * 100
	}

	downloadedMu.Lock()
	downloaded := DownloadedBytes
	total := TotalFileSize
	downloadedMu.Unlock()

	var dataPercentage float64
	if total > 0 {
		dataPercentage = (float64(downloaded) / float64(total)) * 100
	}

	log.Printf("Progress: %d/%d pieces (%.1f%%) | Downloaded: %.2f MB / %.2f MB (%.1f%%) | Peers: %d total, %d active | In progress: %d",
		completedPieces, totalPieces, piecePercentage,
		float64(downloaded)/(1024*1024), float64(total)/(1024*1024), dataPercentage,
		totalPeers, unchokedCount, inProgressPieces)
}

func AddDownloadedBytes(bytes int64) {
	downloadedMu.Lock()
	DownloadedBytes += bytes
	downloadedMu.Unlock()
}

func ResetPieceForRetry(pieceIndex uint32) {
	globalPieceMu.Lock()
	defer globalPieceMu.Unlock()

	piece, exists := GlobalPieceList[pieceIndex]
	if !exists {
		return
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	piece.IsRequested = false
	piece.IsComplete = false
	piece.IsVerified = false
	piece.ReceivedBlocks = make(map[uint32]bool)
	piece.RequestedBlocks = make(map[uint32]bool)
}

func AddHavePiece(ip string, port int, piece uint32) *ClientState {
	globalMu.Lock()
	client, exists := GlobalClientList[ip]
	if !exists {
		client = &ClientState{
			IP:     ip,
			Port:   port,
			Choked: true,
			Pieces: []uint32{piece},
		}
		GlobalClientList[ip] = client
		globalMu.Unlock()
		return client
	}
	globalMu.Unlock()

	client.Mu.Lock()
	defer client.Mu.Unlock()

	for _, p := range client.Pieces {
		if p == piece {
			return client
		}
	}

	client.Pieces = append(client.Pieces, piece)
	return client
}

func (cs *ClientState) IsChoked() bool {
	cs.Mu.Lock()
	defer cs.Mu.Unlock()
	return cs.Choked
}

// HasPiece checks if the client has a specific piece.
func (cs *ClientState) HasPiece(piece uint32) bool {
	cs.Mu.Lock()
	defer cs.Mu.Unlock()
	for _, p := range cs.Pieces {
		if p == piece {
			return true
		}
	}
	return false
}

func (cs *ClientState) SetChoked(choked bool) {
	cs.Mu.Lock()
	defer cs.Mu.Unlock()
	cs.Choked = choked
}

func (ps *PieceState) WriteData(offset uint64, data []byte) {
	ps.Mu.Lock()
	defer ps.Mu.Unlock()

	copy(ps.Buffer[offset:], data)
	ps.NextOffset += uint64(len(data))

	if ps.NextOffset >= ps.TotalLength {
		ps.IsComplete = true
	}
}

func IsPieceComplete(pieceIndex uint32) bool {
	globalPieceMu.RLock()
	defer globalPieceMu.RUnlock()

	piece, exists := GlobalPieceList[pieceIndex]
	if !exists {
		return false
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()
	return piece.IsComplete && piece.IsVerified
}

func IsPieceBeingRequested(pieceIndex uint32) bool {
	globalPieceMu.RLock()
	defer globalPieceMu.RUnlock()

	piece, exists := GlobalPieceList[pieceIndex]
	if !exists {
		return false
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()
	return piece.IsRequested && !piece.IsComplete
}

func MarkPieceAsRequested(pieceIndex uint32) {
	globalPieceMu.RLock()
	piece, exists := GlobalPieceList[pieceIndex]
	globalPieceMu.RUnlock()

	if !exists {
		return
	}

	piece.Mu.Lock()
	piece.IsRequested = true
	piece.Mu.Unlock()
}

func SelectNextPiece(peerIP string) int32 {
	globalMu.RLock()
	client, exists := GlobalClientList[peerIP]
	globalMu.RUnlock()

	if !exists {
		return -1
	}

	client.Mu.Lock()
	peerPieces := make([]uint32, len(client.Pieces))
	copy(peerPieces, client.Pieces)
	client.Mu.Unlock()

	candidates := []uint32{}

	for _, pieceIndex := range peerPieces {
		if IsPieceComplete(pieceIndex) {
			continue
		}

		if IsPieceBeingRequested(pieceIndex) {
			continue
		}

		candidates = append(candidates, pieceIndex)
	}

	if len(candidates) == 0 {
		return -1
	}

	randomIndex := rand.Intn(len(candidates))
	selectedPiece := candidates[randomIndex]

	return int32(selectedPiece)
}

func GetOrCreatePieceState(pieceIndex uint32, pieceLength uint64) *PieceState {
	globalPieceMu.Lock()
	defer globalPieceMu.Unlock()

	piece, exists := GlobalPieceList[pieceIndex]
	if exists {
		return piece
	}

	blockSize := uint32(16384)
	totalBlocks := uint32((pieceLength + uint64(blockSize) - 1) / uint64(blockSize))

	piece = &PieceState{
		PieceNo:         pieceIndex,
		TotalLength:     pieceLength,
		IsRequested:     true,
		IsComplete:      false,
		IsVerified:      false,
		Buffer:          make([]byte, pieceLength),
		BlockSize:       blockSize,
		TotalBlocks:     totalBlocks,
		ReceivedBlocks:  make(map[uint32]bool),
		RequestedBlocks: make(map[uint32]bool),
	}

	GlobalPieceList[pieceIndex] = piece
	return piece
}

func GetPieceState(pieceIndex uint32) (*PieceState, bool) {
	globalPieceMu.RLock()
	defer globalPieceMu.RUnlock()

	piece, exists := GlobalPieceList[pieceIndex]
	return piece, exists
}

func IsPieceFullyReceived(pieceIndex uint32) bool {
	piece, exists := GetPieceState(pieceIndex)
	if !exists {
		return false
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	return uint32(len(piece.ReceivedBlocks)) == piece.TotalBlocks
}

func MarkBlockReceived(pieceIndex, blockIndex uint32) {
	piece, exists := GetPieceState(pieceIndex)
	if !exists {
		return
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	piece.ReceivedBlocks[blockIndex] = true

	if uint32(len(piece.ReceivedBlocks)) == piece.TotalBlocks {
		piece.IsComplete = true
	}
}

func MarkBlockRequested(pieceIndex, blockIndex uint32) {
	piece, exists := GetPieceState(pieceIndex)
	if !exists {
		return
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	piece.RequestedBlocks[blockIndex] = true
}

func IsBlockRequested(pieceIndex, blockIndex uint32) bool {
	piece, exists := GetPieceState(pieceIndex)
	if !exists {
		return false
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	return piece.RequestedBlocks[blockIndex]
}

func IsBlockReceived(pieceIndex, blockIndex uint32) bool {
	piece, exists := GetPieceState(pieceIndex)
	if !exists {
		return false
	}

	piece.Mu.Lock()
	defer piece.Mu.Unlock()

	return piece.ReceivedBlocks[blockIndex]
}
