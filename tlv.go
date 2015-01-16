package goutils

import (
	"bytes"
	"encoding/binary"
)

const (
	TLVHeaderLen = 4
)

type TLVHeader struct {
	TLVType    uint8
	TLVSubtype uint8
	TLVLength  uint16
}
type tlvError struct {
	errorType string
}

func (tlvErr *tlvError) Error() string {
	return tlvErr.errorType
}

func (tlv TLVHeader) Encode() ([]byte, error) {
	buffer := new(bytes.Buffer)
	err := binary.Write(buffer, binary.BigEndian, &tlv)
	if err != nil {
		return nil, &tlvError{"cant encode tlv"}
	}
	return buffer.Bytes(), nil
}

func (tlv *TLVHeader) Decode(buffer []byte) error {
	reader := bytes.NewReader(buffer)
	err := binary.Read(reader, binary.BigEndian, tlv)
	if err != nil {
		return &tlvError{"cant decode tlv"}
	}
	return nil
}

func GenerateTLV(tlvType, tlvSubtype uint8, data []byte) ([]byte, error) {
	tlvHeader := TLVHeader{tlvType, tlvSubtype, 0}
	tlvHeader.TLVLength = uint16(len(data) + TLVHeaderLen)
	encodedHeader, err := tlvHeader.Encode()
	if err != nil {
		return nil, err
	}
	msg := append(encodedHeader, data...)
	return msg, nil
}
