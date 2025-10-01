package main

import (
	"encoding/binary"
	"io"
)

type pixelFormat struct {
	BPP, Depth             uint8
	BigEndian, TrueColor   uint8
	RMax, GMax, BMax       uint16
	RShift, GShift, BShift uint8
	_                      [3]byte // padding
}

func write16(dst io.Writer, v ...uint16) error {
	for _, x := range v {
		err := binary.Write(dst, binary.BigEndian, x)
		if err != nil {
			return err
		}
	}
	return nil
}

func write32(dst io.Writer, v uint32) error {
	return binary.Write(dst, binary.BigEndian, v)
}
