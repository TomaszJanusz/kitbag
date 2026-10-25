package embedded

import (
	"context"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
)

// AdvertisedReferences delegates with a background context.
func (s *localUpload) AdvertisedReferences() (*packp.AdvRefs, error) {
	return s.AdvertisedReferencesContext(context.Background())
}

// AdvertisedReferencesContext advertises the shallow support supplied by this adapter.
func (s *localUpload) AdvertisedReferencesContext(ctx context.Context) (*packp.AdvRefs, error) {
	refs, err := s.UploadPackSession.AdvertisedReferencesContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = refs.Capabilities.Set(capability.Shallow); err != nil {
		return nil, err
	}
	return refs, nil
}

// The local Go server transfers full object packs; the client still records
// depth boundaries, giving local and network clones the same history semantics.
func (s *localUpload) shallowBoundaries(ctx context.Context, req *packp.UploadPackRequest) ([]plumbing.Hash, error) {
	depth, ok := req.Depth.(packp.DepthCommits)
	if !ok || depth == 0 {
		return req.Shallows, nil
	}
	frontier := req.Wants
	seen := map[plumbing.Hash]bool{}
	for level := 1; level < int(depth); level++ {
		var next []plumbing.Hash
		for _, hash := range frontier {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if seen[hash] {
				continue
			}
			seen[hash] = true
			c, err := object.GetCommit(s.store, hash)
			if err != nil {
				return nil, err
			}
			next = append(next, c.ParentHashes...)
		}
		frontier = next
	}
	return frontier, nil
}
