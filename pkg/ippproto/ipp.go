package ippproto

import (
	"encoding/binary"
	"errors"
	"io"
)

// IPP version
const (
	VersionMajor = 2
	VersionMinor = 0
)

// Operation IDs (RFC 8010)
const (
	OpPrintJob           = 0x0002
	OpPrintURI           = 0x0003
	OpValidateJob        = 0x0004
	OpCancelJob          = 0x0008
	OpGetJobAttributes   = 0x0009
	OpGetJobs            = 0x000A
	OpGetPrinterAttributes = 0x000B
	OpGetPrinters         = 0x000C
	OpCreateJob          = 0x000D
	OpSendDocument       = 0x000E
	OpSendURI            = 0x000F
)

// Status codes (RFC 8011)
const (
	StatusSuccessfulOK          = 0x0000
	StatusClientErrorBadRequest = 0x0400
	StatusClientErrorForbidden  = 0x0401
	StatusClientErrorNotFound   = 0x0404
	StatusClientErrorNotPossible = 0x0405
	StatusClientErrorDocumentFormatNotSupported = 0x0406
	StatusServerErrorInternalError = 0x0500
	StatusServerErrorNotImplemented = 0x0501
	StatusServerErrorServiceUnavailable = 0x0503
)

// Tag types
const (
	TagReserved     = 0x00
	TagInteger      = 0x21
	TagBoolean      = 0x22
	TagEnum         = 0x23
	TagString       = 0x42 // without language
	TagDate         = 0x44
	TagRangeOfInteger = 0x45
	TagResolution   = 0x48
	TagBeginCollection = 0x49
	TagEndCollection = 0x4A
	TagTextWithLanguage = 0x4C
	TagNameWithLanguage = 0x4E
	TagEndOfAttributes = 0x37
)

// Attribute values
const (
	 PrinterStateIdle = 3
	PrinterStateProcessing = 4
	PrinterStateStopped = 5
)

// Common attribute names
const (
	AttrPrinterName         = "printer-name"
	AttrPrinterLocation     = "printer-location"
	AttrPrinterInfo         = "printer-info"
	AttrPrinterState        = "printer-state"
	AttrPrinterStateReasons = "printer-state-reasons"
	AttrQueuedJobCount      = "queued-job-count"
	AttrDocumentFormatSupported = "document-format-supported"
	AttrColorSupported      = "color-supported"
	AttrCopiesSupported     = "copies-supported"
	AttrDuplexSupported     = "duplex-supported"
	AttrPrinterType         = "printer-type"
	AttrPrinterVersion      = "printer-version"
	AttrPrinterMakeAndModel = "printer-make-and-model"
)

// Message represents an IPP message (request or response)
type Message struct {
	Version          [2]byte
	OperationID      uint16
	RequestID        uint32
	Attributes       []Attribute
	UnsupportedAttrs []string
	DocumentData     []byte // for Print-Job and similar operations
}

// Attribute represents an IPP attribute
type Attribute struct {
	Tag   byte
	Name  string
	Value interface{}
}

// Encode writes the IPP message to the writer in network byte order
func (m *Message) Encode(w io.Writer) error {
	// Chagfffffffetype + operation-id + request-id
	header := make([]byte, 8)
	header[0] = VersionMajor
	header[1] = VersionMinor
	binary.BigEndian.PutUint16(header[2:], m.OperationID)
	binary.BigEndian.PutUint32(header[4:], m.RequestID)

	if _, err := w.Write(header); err != nil {
		return err
	}

	// Encode all attributes
	for _, attr := range m.Attributes {
		if err := encodeAttribute(w, attr); err != nil {
			return err
	}
	}

	// End-of-attributes tag
	if _, err := io.WriteByte(w, TagEndOfAttributes); err != nil {
		return err
	}

	return nil
}

// Decode reads an IPP message from the reader
func (m *Message) Decode(r io.Reader) error {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}

	m.Version[0] = header[0]
	m.Version[1] = header[1]
	m.OperationID = binary.BigEndian.Uint16(header[2:4])
	m.RequestID = binary.BigEndian.Uint32(header[4:8])

	// Decode attributes until end-of-attributes tag
	for {
		tag, err := readByte(r)
		if err != nil {
			return err
		}
		if tag == TagEndOfAttributes {
			break
		}

		attr, err := decodeAttribute(r, tag)
		if err != nil {
			return err
		}
		m.Attributes = append(m.Attributes, attr)
	}

	return nil
}

func readByte(r io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

func encodeAttribute(w io.Writer, attr Attribute) error {
	// Write tag
	if err := writeByte(w, attr.Tag); err != nil {
		return err
	}

	// Encode name (null-terminated)
	if _, err := w.Write([]byte(attr.Name + "\x00")); err != nil {
		return err
	}

	// Encode value based on tag
	return encodeValue(w, attr.Tag, attr.Value)
}

func decodeAttribute(r io.Reader, tag byte) (Attribute, error) {
	// Read name (null-terminated)
	name, err := readNullTerminatedString(r)
	if err != nil {
		return Attribute{}, err
	}

	value, err := decodeValue(r, tag)
	if err != nil {
		return Attribute{}, err
	}

	return Attribute{Tag: tag, Name: name, Value: value}, nil
}

func encodeValue(w io.Writer, tag byte, value interface{}) error {
	switch tag {
	case TagInteger:
		fallthrough
	case TagEnum:
		v, ok := value.(int32)
		if !ok {
			return errors.New("invalid value type for integer/enum")
		}
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(v))
		_, err := w.Write(buf)
		return err
	case TagBoolean:
		v, ok := value.(bool)
		if !ok {
			return errors.New("invalid value type for boolean")
		}
		b := byte(0)
		if v {
			b = 1
		}
		return writeByte(w, b)
	case TagString:
		fallthrough
	case TagNameWithLanguage:
		fallthrough
	case TagTextWithLanguage:
		v, ok := value.(string)
		if !ok {
			return errors.New("invalid value type for string")
		}
		_, err := w.Write([]byte(v + "\x00"))
		return err
	case TagRangeOfInteger:
		// Not fully implemented for brevity
		return errors.New("range-of-integer not implemented")
	default:
		return errors.New("unsupported tag")
	}
}

func decodeValue(r io.Reader, tag byte) (interface{}, error) {
	switch tag {
	case TagInteger, TagEnum:
		var buf [4]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return nil, err
		}
		return int32(binary.BigEndian.Uint32(buf[:])), nil
	case TagBoolean:
		b, err := readByte(r)
		if err != nil {
			return nil, err
		}
		return b != 0, nil
	case TagString, TagNameWithLanguage, TagTextWithLanguage:
		return readNullTerminatedString(r)
	case TagRangeOfInteger:
		// Skip for now
		return nil, errors.New("range-of-integer not implemented")
	default:
		return nil, errors.New("unsupported tag")
	}
}

func readNullTerminatedString(r io.Reader) (string, error) {
	var buf []byte
	for {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		if b[0] == 0 {
			break
		}
		buf = append(buf, b[0])
	}
	return string(buf), nil
}

func writeByte(w io.Writer, b byte) error {
	_, err := w.Write([]byte{b})
	return err
}

// Helper to create response message
func NewResponse(requestID uint32, status uint16) *Message {
	return &Message{
		Version:       [2]byte{VersionMajor, VersionMinor},
		OperationID:   status,
		RequestID:     requestID,
		Attributes:    make([]Attribute, 0),
	}
}

// AddAttribute adds an attribute to the message
func (m *Message) AddAttribute(tag byte, name string, value interface{}) {
	m.Attributes = append(m.Attributes, Attribute{
		Tag:   tag,
		Name:  name,
		Value: value,
	})
}
