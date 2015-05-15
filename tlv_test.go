package goutils

import (
	"testing"
)

func TestTLV(t *testing.T) {
	tlv := TLVHeader{TLVType: uint8(223), TLVSubtype: uint8(133)}
	tlve, err := tlv.Encode()
	if err != nil {
		t.Errorf("error in tlv encoding\n")
	}
	tlv2 := TLVHeader{}
	err = tlv2.Decode(tlve)
	if err != nil {
		t.Errorf("cant decode encoded tlv\n")
	}
	if tlv.TLVType != tlv2.TLVType || tlv.TLVSubtype != tlv2.TLVSubtype {
		t.Errorf("decoded tlv not equal to encoded")
	}
}
