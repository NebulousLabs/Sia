package consensus

import (
	"github.com/NebulousLabs/Sia/encoding"
	"github.com/NebulousLabs/Sia/types"
)

type (
	// blockMarshaller encodes a Block for storage.
	blockMarshaller interface {
		Marshal(types.Block) []byte
	}

	// processedBlockUnmarshaller decodes a processedBlock from a byte slice.
	processedBlockUnmarshaller interface {
		Unmarshal([]byte, *processedBlock) error
	}

	// stdBlockMarshaller is an implementation of blockMarshaller that uses the
	// Sia/encoding package to encode a Block.
	stdBlockMarshaller struct{}

	// stdProcessedBlockUnmarshaller is an implementation of
	// processedBlockUnmarshaller that uses the Sia/encoding package to decode
	// a processedBlock from a byte slice.
	stdProcessedBlockUnmarshaller struct{}
)

func (m stdBlockMarshaller) Marshal(block types.Block) []byte {
	return encoding.Marshal(block)
}

func (um stdProcessedBlockUnmarshaller) Unmarshal(blockBytes []byte, block *processedBlock) error {
	err := encoding.Unmarshal(blockBytes, block)
	if err != nil {
		return err
	}
	return nil
}
