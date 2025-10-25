package protocol

// ParseBitfield converts a byte slice into a slice of booleans
// where each element indicates whether the peer has the corresponding piece.
func ParseBitfield(data []byte) []bool {
	bitfield := make([]bool, len(data)*8)
	for i := 0; i < len(data); i++ {
		for j := 0; j < 8; j++ {
			bitfield[i*8+j] = (data[i]&(1<<(7-j)) != 0)
		}
	}
	return bitfield
}

// piecesPeerHas returns a slice of piece indices that the peer has
func PiecesPeerHas(data []byte) []uint32 {
	bitfield := ParseBitfield(data)
	pieces := []uint32{}
	for i, hasPiece := range bitfield {
		if hasPiece {
			pieces = append(pieces, uint32(i))
		}
	}
	return pieces
}
