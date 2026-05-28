// Package uavprotocol provides unified entry points for detector protocol parsing.
package uavprotocol

import (
	"time"

	"uav-protocol/model"
	"uav-protocol/parser"
)

type Parser struct {
	text *parser.Parser
}

type Options struct {
	Now func() time.Time
}

func NewParser(opts Options) *Parser {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Parser{
		text: parser.New(parser.Options{Now: now}),
	}
}

func ParseText(line string) (*model.Message, error) {
	return NewParser(Options{}).ParseText(line)
}

func (p *Parser) ParseText(line string) (*model.Message, error) {
	if p == nil || p.text == nil {
		p = NewParser(Options{})
	}
	return p.text.ParseLine(line)
}

func ParseSpectrum(data []byte) (*model.Message, bool) {
	return NewParser(Options{}).ParseSpectrum(data)
}

func (p *Parser) ParseSpectrum(data []byte) (*model.Message, bool) {
	if p == nil || p.text == nil {
		p = NewParser(Options{})
	}
	return p.text.ParseSpectrum(data)
}

func ParseHex(payload string) (*model.Message, error) {
	return NewParser(Options{}).ParseHex(payload)
}

func (p *Parser) ParseHex(payload string) (*model.Message, error) {
	if p == nil || p.text == nil {
		p = NewParser(Options{})
	}
	return p.text.ParseHex(payload)
}

func ParseBytes(data []byte) (*model.Message, bool, error) {
	return NewParser(Options{}).ParseBytes(data)
}

func (p *Parser) ParseBytes(data []byte) (*model.Message, bool, error) {
	if p == nil || p.text == nil {
		p = NewParser(Options{})
	}
	return p.text.ParseBytes(data)
}
