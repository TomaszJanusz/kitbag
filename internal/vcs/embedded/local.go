package embedded

import (
	"context"
	"errors"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
)

// localLoader also handles non-bare repositories and permission errors.
type localLoader struct{}

// Load opens exactly the endpoint repository.
func (localLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	r, err := git.PlainOpen(ep.Path)
	if err != nil {
		return nil, err
	}
	return r.Storer, nil
}

type localClient struct{ transport.Transport }

// NewUploadPackSession adapts the in-process server for client fetch negotiation.
func (c localClient) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	session, err := c.Transport.NewUploadPackSession(ep, auth)
	if err != nil {
		return nil, err
	}
	store, err := (localLoader{}).Load(ep)
	if err != nil {
		return nil, errors.Join(err, session.Close())
	}
	return &localUpload{UploadPackSession: session, store: store}, nil
}

type localUpload struct {
	transport.UploadPackSession
	store storer.Storer
}

// UploadPack filters unknown haves and applies requested shallow boundaries.
func (s *localUpload) UploadPack(ctx context.Context, req *packp.UploadPackRequest) (*packp.UploadPackResponse, error) {
	// A client may have private commits the server does not know. go-git's
	// server walks all haves as objects, so filter unknown tips before handing
	// over the request. Omitting a have only sends a larger pack, never less data.
	filtered := make([]plumbing.Hash, 0, len(req.Haves))
	for _, hash := range req.Haves {
		err := s.store.HasEncodedObject(hash)
		if err == nil {
			filtered = append(filtered, hash)
		} else if !errors.Is(err, plumbing.ErrObjectNotFound) {
			return nil, err
		}
	}
	copyReq := *req
	copyReq.Haves = filtered
	copyReq.Shallows = nil
	copyReq.Depth = packp.DepthCommits(0)
	// The underlying session saw our augmented advertised capabilities too.
	// Remove shallow from the wire request: boundaries are applied below.
	copyReq.Capabilities = capability.NewList()
	for _, cap := range req.Capabilities.All() {
		if cap == capability.Shallow {
			continue
		}
		if err := copyReq.Capabilities.Set(cap, req.Capabilities.Get(cap)...); err != nil {
			return nil, err
		}
	}
	boundaries, err := s.shallowBoundaries(ctx, req)
	if err != nil {
		return nil, err
	}
	response, err := s.UploadPackSession.UploadPack(ctx, &copyReq)
	if err != nil {
		return nil, err
	}
	response.Shallows = boundaries
	return response, nil
}

func newLocalClient() transport.Transport {
	return localClient{Transport: server.NewClient(localLoader{})}
}
