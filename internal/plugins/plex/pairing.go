package plex

import (
	"context"
	"errors"
)

// ErrPairingUnavailable is returned until the plex.tv PIN flow is built; pasting a token works now.
var ErrPairingUnavailable = errors.New("plex PIN pairing is not available yet; paste a token")

// StartPinPairing is the `pinPairing` setup flow.
//
// TODO: Plex PIN pairing flow (plex.tv PIN request and poll) is not built; until then pasting a token is the only way to pair.
func (s *Server) StartPinPairing(context.Context) error { return ErrPairingUnavailable }
