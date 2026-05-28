package parser

import "uav-protocol/model"

type MessageType = model.MessageType
type Message = model.Message
type GPS = model.GPS
type DIDEncrypted = model.DIDEncrypted
type RID = model.RID
type DIDPlain = model.DIDPlain
type Detect = model.Detect
type Heartbeat = model.Heartbeat
type Empty = model.Empty

const (
	TypeUnknown      = model.TypeUnknown
	TypeDIDEncrypted = model.TypeDIDEncrypted
	TypeRID          = model.TypeRID
	TypeDIDPlain     = model.TypeDIDPlain
	TypeDetect       = model.TypeDetect
	TypeHeartbeat    = model.TypeHeartbeat
	TypeEmpty        = model.TypeEmpty
	TypeSpectrum     = model.TypeSpectrum
)
