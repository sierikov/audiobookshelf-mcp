package service

import (
	"context"
)

// Items looks up single items and what the user is listening to.
type Items struct {
	api ItemsAPI
}

// NewItems creates an Items service backed by api.
func NewItems(api ItemsAPI) *Items {
	return &Items{api: api}
}

// Get returns an item with all of its metadata. Media that cannot be decoded
// is reported as a [*MediaError].
func (s *Items) Get(ctx context.Context, itemID string) (ItemDetail, error) {
	li, err := s.api.Item(ctx, itemID)
	if err != nil {
		return ItemDetail{}, err
	}
	return toItemDetail(li)
}

// InProgress returns the items the user is listening to, each with its
// progress. A progress lookup that fails leaves that item's Progress nil.
func (s *Items) InProgress(ctx context.Context, limit int) ([]InProgressItem, error) {
	lis, err := s.api.ItemsInProgress(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]InProgressItem, 0, len(lis))
	for _, li := range lis {
		it, err := toItem(li)
		if err != nil {
			continue
		}
		out = append(out, InProgressItem{Item: it, Progress: s.progress(ctx, li.ID)})
	}
	return out, nil
}

func (s *Items) progress(ctx context.Context, itemID string) *Progress {
	p, err := s.api.Progress(ctx, itemID, "")
	if err != nil {
		return nil
	}
	prog := toProgress(p)
	return &prog
}
