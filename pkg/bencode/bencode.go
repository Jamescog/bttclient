package bencode

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// Torrent represents a decoded torrent file
type Torrent struct {
	Data map[string]interface{}
}

// DecodeNext parses the next bencoded value starting at `index`.
// Returns value (int, string, []interface{}, map[string]interface{}) and next index.
func DecodeNext(data []byte, index int) (interface{}, int, error) {
	if index >= len(data) {
		return nil, index, fmt.Errorf("unexpected end of data")
	}

	char := data[index]

	if char == 'i' {
		end := bytes.IndexByte(data[index:], 'e')
		if end == -1 {
			return nil, index, fmt.Errorf("unterminated integer")
		}
		end += index
		num, err := strconv.Atoi(string(data[index+1 : end]))
		if err != nil {
			return nil, end + 1, err
		}
		return num, end + 1, nil
	}

	if char >= '0' && char <= '9' {
		colon := bytes.IndexByte(data[index:], ':')
		if colon == -1 {
			return nil, index, fmt.Errorf("missing colon in string")
		}
		colon += index
		length, err := strconv.Atoi(string(data[index:colon]))
		if err != nil {
			return nil, colon + 1, err
		}
		start := colon + 1
		end := start + length
		if end > len(data) {
			return nil, len(data), fmt.Errorf("string length exceeds data length")
		}
		value := data[start:end]
		return value, end, nil
	}

	if char == 'l' {
		var lst []interface{}
		i := index + 1
		for i < len(data) && data[i] != 'e' {
			item, next, err := DecodeNext(data, i)
			if err != nil {
				return nil, i, err
			}
			lst = append(lst, item)
			i = next
		}
		if i >= len(data) || data[i] != 'e' {
			return nil, i, fmt.Errorf("unterminated list")
		}
		return lst, i + 1, nil
	}

	if char == 'd' {
		dct := make(map[string]interface{})
		i := index + 1
		for i < len(data) && data[i] != 'e' {
			keyRaw, next, err := DecodeNext(data, i)
			if err != nil {
				return nil, i, err
			}
			keyBytes, ok := keyRaw.([]byte)
			if !ok {
				return nil, i, fmt.Errorf("dictionary keys must be strings")
			}
			key := string(keyBytes)
			i = next

			value, next, err := DecodeNext(data, i)
			if err != nil {
				return nil, i, err
			}
			dct[key] = value
			i = next
		}
		if i >= len(data) || data[i] != 'e' {
			return nil, i, fmt.Errorf("unterminated dictionary")
		}
		return dct, i + 1, nil
	}

	return nil, index, fmt.Errorf("unknown prefix: %c", char)
}

// toReadable converts byte slices to strings recursively for human-readable output
func toReadable(v interface{}) interface{} {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = toReadable(item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			result[k] = toReadable(v)
		}
		return result
	default:
		return v
	}
}

// DecodeTorrentFile reads a torrent file and decodes its bencoding
func DecodeTorrentFile(path string) (*Torrent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	value, _, err := DecodeNext(data, 0)
	if err != nil {
		return nil, err
	}

	readable := toReadable(value)
	m, ok := readable.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("decoded value is not a map")
	}

	return &Torrent{Data: m}, nil
}

// Announce returns the announce URL
func (t *Torrent) Announce() string {
	if val, ok := t.Data["announce"].(string); ok {
		return val
	}
	return ""
}

// Info returns the info dictionary
func (t *Torrent) Info() map[string]interface{} {
	if val, ok := t.Data["info"].(map[string]interface{}); ok {
		return val
	}
	return nil
}

// Name returns the torrent name from info
func (t *Torrent) Name() string {
	info := t.Info()
	if info != nil {
		if val, ok := info["name"].(string); ok {
			return val
		}
	}
	return ""
}

// PieceLength returns the piece length from info
func (t *Torrent) PieceLength() int {
	info := t.Info()
	if info != nil {
		if val, ok := info["piece length"].(int); ok {
			return val
		}
	}
	return 0
}
