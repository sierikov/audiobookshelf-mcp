package service

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
	"github.com/sierikov/audiobookshelf-mcp/internal/gemini"
)

const (
	// sourceWidthPro and sourceWidthStandard are the widths, in pixels, of the
	// images the Pro model (2K) and the standard model (1K) make; see
	// [sourceCoverWidth].
	sourceWidthPro      = 2048
	sourceWidthStandard = 1024
	// keptImages is the most made images kept at once; a new one pushes out
	// the oldest.
	keptImages = 24
	// keepImagesFor is how long a made image is kept.
	keepImagesFor = time.Hour
)

// tokenPrices is what Google bills for a token of an image model, in US
// dollars: the input, text and images, the image made, and the thinking.
type tokenPrices struct {
	prompt, output, thought float64
}

// modelPrices are the prices of each image model, by the model's ID. The
// standard model bills $1.50 a million for the input, $30 a million for the
// image made and $7.50 a million for the thinking; Pro bills $2 a million, $120
// a million and $12 a million.
var modelPrices = map[gemini.Model]tokenPrices{
	gemini.ModelStandard: {prompt: 1.50 / 1e6, output: 30.0 / 1e6, thought: 7.50 / 1e6},
	gemini.ModelPro:      {prompt: 2.00 / 1e6, output: 120.0 / 1e6, thought: 12.0 / 1e6},
}

// sourceCoverWidth is the width, in pixels, a cover that goes to model is
// fetched at, the cover to clean or a reference: the width of the images the
// model makes, 2048 for [gemini.ModelPro] and 1024 for any other, so that a
// cleaning or a reference does not start from less than it makes.
func sourceCoverWidth(model gemini.Model) int {
	if model == gemini.ModelPro {
		return sourceWidthPro
	}
	return sourceWidthStandard
}

// The problems of a cover that was not made, besides a refusal's.
const (
	problemStopped  = "not made: the call stopped"
	problemTimedOut = "timed out"
	problemNotMade  = "could not be made"
)

// CoverArt makes new covers for books and cleans their current ones with an
// image model, keeps what it made in memory for [keepImagesFor], counts what
// making it cost, and sets a made image as a book's cover. Nothing is written
// to disk: the images and the count are lost when the server stops.
type CoverArt struct {
	api   CoverArtAPI
	model ImageModel
	// preview makes the preview of an image; tests replace it.
	preview func(abs.Image) (abs.Image, error)
	// slots has a place for each preview being made, [maxConcurrentPreviews]
	// of them, for all calls together.
	slots chan struct{}
	// now is the clock the kept images age by; tests replace it.
	now func() time.Time

	mu sync.Mutex
	// kept are the made images, oldest first: at most [keptImages], none
	// older than [keepImagesFor] once [CoverArt.dropExpired] has run.
	kept []keptImage
	// made is the number of images made, and used the tokens all requests
	// used, refusals included, by the model they were made with.
	made int
	used map[gemini.Model]gemini.Usage
}

// keptImage is a made image, the book it was made for and when.
type keptImage struct {
	id     string
	itemID string
	image  abs.Image
	made   time.Time
}

// coverJob is one cover to make: the request for the model and the book and
// style it is for.
type coverJob struct {
	itemID, title, style string
	req                  gemini.Request
}

// NewCoverArt creates a CoverArt service that talks to Audiobookshelf through
// api and makes images with model.
func NewCoverArt(api CoverArtAPI, model ImageModel) *CoverArt {
	return &CoverArt{
		api: api, model: model,
		preview: preview, slots: make(chan struct{}, maxConcurrentPreviews), now: time.Now,
		used: map[gemini.Model]gemini.Usage{},
	}
}

// Generate makes the covers r asks for with the image model r.Model names, all
// at once under one context, each from the prompt [newCoverPrompt] writes from
// the book's title, author and genres, the cover's style and premise and
// r.TitleText. Every cover is square ([squareRatio]), as audiobook covers are.
// A cover's Title, when it has one, is the title in the prompt in place of the
// book's own. With a reference, each request carries that image and the prompt
// asks to match its style.
//
// Checked before any call: the number of covers ([ErrNewCoverCount]), at most
// one reference ([ErrOneReference]), a model that is empty (Pro), "pro" or
// "standard" ([ErrUnknownModel]), each style, required, premise and title,
// plain text within [MaxStyleLength], [MaxPremiseLength] and [MaxTitleLength]
// once trimmed, and that a ReferenceImageID is kept ([ErrUnknownImage]). Then,
// before any image is made, the reference book's cover is read, at
// [sourceCoverWidth] for the model ([ErrNoReferenceCover] when it has none),
// and each book once with its current cover. An error there, or an item that
// is not a book ([ErrBooksOnly]), fails the call.
//
// A cover that is not made has a Problem and does not fail the call: Google
// refused it, it timed out or Google answered with an error status. The first
// error that says every further request would fail too, such as a spending
// limit, stops the call: the requests still running are cancelled, nothing is
// retried, and Stopped says why. The covers already made are kept. If ctx
// ends first, its error is returned.
func (a *CoverArt) Generate(ctx context.Context, r GenerateRequest) (CoverArtResult, error) {
	covers, err := checkNewCovers(r.Covers)
	if err != nil {
		return CoverArtResult{}, err
	}
	if r.ReferenceImageID != "" && r.ReferenceItemID != "" {
		return CoverArtResult{}, ErrOneReference
	}
	model, err := r.Model.imageModel()
	if err != nil {
		return CoverArtResult{}, err
	}
	refs, err := a.reference(ctx, r.ReferenceImageID, r.ReferenceItemID, sourceCoverWidth(model))
	if err != nil {
		return CoverArtResult{}, err
	}
	books, current, err := a.readBooks(ctx, covers)
	if err != nil {
		return CoverArtResult{}, err
	}
	jobs := make([]coverJob, len(covers))
	for i, c := range covers {
		b := books[c.ItemID]
		onCover := b
		onCover.Title = cmp.Or(c.Title, onCover.Title) // the prompt's title; the result keeps the book's own
		jobs[i] = coverJob{itemID: c.ItemID, title: b.Title, style: c.Style, req: gemini.Request{
			Model:       model,
			Prompt:      newCoverPrompt(onCover, c.Style, c.Premise, r.TitleText, refs != nil),
			Images:      refs,
			AspectRatio: squareRatio,
		}}
	}
	return a.makeCovers(ctx, current, jobs)
}

// Clean has the image model that model names (empty is Pro) change a book's
// current cover as instructions say, keeping the rest, with the prompt
// [editCoverPrompt] writes and the cover at [sourceCoverWidth] for the model,
// 2048 px for Pro and 1024 px for the standard one. The cleaned cover is square
// ([squareRatio]), whatever the shape of the cover it starts from: that is both
// the aspect ratio asked of the model and what the prompt asks for. The
// instructions must be plain text within [MaxEditLength] once trimmed, and the
// model empty, "pro" or "standard" ([ErrUnknownModel]); both are checked before
// any call. An item that is not a book is an [ErrBooksOnly] error, and a book
// without a cover an [ErrNoCoverToClean] error; so is a cover that can't be
// previewed, and nothing is spent on it. The result has the current cover, as a
// preview, and the cleaned one; a cleaning that is not made or stops is
// reported as for [CoverArt.Generate].
func (a *CoverArt) Clean(ctx context.Context, itemID, instructions string, model CoverModel) (CoverArtResult, error) {
	instructions = strings.TrimSpace(instructions)
	if reason := badValue("instructions", instructions, MaxEditLength); reason != "" {
		return CoverArtResult{}, errors.New(reason)
	}
	imageModel, err := model.imageModel()
	if err != nil {
		return CoverArtResult{}, err
	}
	li, err := a.api.Item(ctx, itemID)
	if err != nil {
		return CoverArtResult{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return CoverArtResult{}, err
	}
	cover, err := a.api.ItemCover(ctx, itemID, sourceCoverWidth(imageModel))
	switch {
	case isNotFound(err):
		return CoverArtResult{}, fmt.Errorf("item %s: %w", itemID, ErrNoCoverToClean)
	case err != nil:
		return CoverArtResult{}, fmt.Errorf("reading the cover of item %s: %w", itemID, err)
	}
	shown, err := a.previewWhenFree(ctx, cover)
	if err != nil {
		// Nothing is spent on a cover that can't be looked at.
		return CoverArtResult{}, fmt.Errorf("item %s: the current cover can't be shown: %w", itemID, err)
	}
	title := b.Metadata.Title
	job := coverJob{itemID: itemID, title: title, req: gemini.Request{
		Model: imageModel, Prompt: editCoverPrompt(instructions), Images: []gemini.Image{gemini.Image(cover)},
		AspectRatio: squareRatio,
	}}
	return a.makeCovers(ctx, []CurrentCover{{ItemID: itemID, Title: title, Image: &shown}}, []coverJob{job})
}

// Set uploads the made image imageID as the cover of the book itemID, in place
// of the old one. This is final: Audiobookshelf deletes the old cover file.
// Checked before any call: the image must still be kept ([ErrUnknownImage])
// and must have been made for that book ([ErrImageForOtherBook]). The item
// must be a book ([ErrBooksOnly]); a 403 then is an [ErrNoCoverPermission]
// error, as for [Covers.Set].
func (a *CoverArt) Set(ctx context.Context, itemID, imageID string) (CoverSet, error) {
	img, err := a.lookup(imageID)
	if err != nil {
		return CoverSet{}, fmt.Errorf("imageId %s: %w", quoted(imageID), err)
	}
	if img.itemID != itemID {
		return CoverSet{}, fmt.Errorf("imageId %s: %w", quoted(imageID), ErrImageForOtherBook)
	}
	li, err := a.api.Item(ctx, itemID)
	if err != nil {
		return CoverSet{}, err
	}
	b, err := asBook(li)
	if err != nil {
		return CoverSet{}, err
	}
	if err := a.api.SetCoverFile(ctx, itemID, img.image); err != nil {
		return CoverSet{}, coverSetError(itemID, err)
	}
	return CoverSet{ItemID: itemID, Title: b.Metadata.Title}, nil
}

// checkNewCovers checks the covers asked for and returns them with their
// style, premise and title trimmed. There must be 1 to [MaxNewCovers]; each
// must have a style, and a style, a premise and a title must be plain text
// within their limits. The error names the cover by its place, from 1.
func checkNewCovers(covers []NewCoverRequest) ([]NewCoverRequest, error) {
	if len(covers) == 0 || len(covers) > MaxNewCovers {
		return nil, fmt.Errorf("%w, not %d", ErrNewCoverCount, len(covers))
	}
	out := make([]NewCoverRequest, len(covers))
	for i, c := range covers {
		c.Style, c.Premise, c.Title = strings.TrimSpace(c.Style), strings.TrimSpace(c.Premise), strings.TrimSpace(c.Title)
		reason := badValue("style", c.Style, MaxStyleLength)
		if reason == "" && c.Premise != "" {
			reason = badValue("premise", c.Premise, MaxPremiseLength)
		}
		if reason == "" && c.Title != "" {
			reason = badValue("title", c.Title, MaxTitleLength)
		}
		if reason != "" {
			return nil, fmt.Errorf("cover %d: %s", i+1, reason)
		}
		out[i] = c
	}
	return out, nil
}

// reference returns the image the new covers match, as the images of a
// request: the kept image imageID, or the current cover of the book itemID at
// width pixels (see [sourceCoverWidth]), or nil for neither. At most one is
// given.
func (a *CoverArt) reference(ctx context.Context, imageID, itemID string, width int) ([]gemini.Image, error) {
	switch {
	case imageID != "":
		img, err := a.lookup(imageID)
		if err != nil {
			return nil, fmt.Errorf("referenceImageId %s: %w", quoted(imageID), err)
		}
		return []gemini.Image{gemini.Image(img.image)}, nil
	case itemID != "":
		cover, err := a.api.ItemCover(ctx, itemID, width)
		switch {
		case isNotFound(err):
			return nil, fmt.Errorf("referenceItemId %s: %w", itemID, ErrNoReferenceCover)
		case err != nil:
			return nil, fmt.Errorf("reading the cover of the reference book %s: %w", itemID, err)
		}
		return []gemini.Image{gemini.Image(cover)}, nil
	}
	return nil, nil
}

// readBooks reads each book the covers are for once, with its current cover
// (see [currentCover]). It returns the books by item ID, and their current
// covers in the order the books first appear.
func (a *CoverArt) readBooks(ctx context.Context, covers []NewCoverRequest) (map[string]coverBook, []CurrentCover, error) {
	books := make(map[string]coverBook, len(covers))
	var current []CurrentCover
	for _, c := range covers {
		if _, ok := books[c.ItemID]; ok {
			continue
		}
		li, err := a.api.Item(ctx, c.ItemID)
		if err != nil {
			return nil, nil, err
		}
		b, err := asBook(li)
		if err != nil {
			return nil, nil, err
		}
		img, problem, err := currentCover(ctx, a.api, c.ItemID)
		if err != nil {
			return nil, nil, err
		}
		m := b.Metadata
		books[c.ItemID] = coverBook{Title: m.Title, Author: m.AuthorName, Genres: m.Genres}
		current = append(current, CurrentCover{ItemID: c.ItemID, Title: m.Title, Image: img, Problem: problem})
	}
	return books, current, nil
}

// makeCovers makes the covers of jobs, all at once, and returns them, in the
// order of jobs, after current and with the spend. The requests share a
// context that the first error that stops a batch cancels, with that error as
// the cause, which Stopped then describes. If ctx ends, its error is returned.
func (a *CoverArt) makeCovers(ctx context.Context, current []CurrentCover, jobs []coverJob) (CoverArtResult, error) {
	run, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	covers := make([]NewCover, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Go(func() { covers[i] = a.makeCover(ctx, run, stop, j) })
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return CoverArtResult{}, err // the problems would only say the call was cut short
	}
	out := CoverArtResult{Current: current, Covers: covers, Spent: a.spend()}
	if status, ok := errors.AsType[*gemini.StatusError](context.Cause(run)); ok {
		out.Stopped = stoppedMessage(status)
	}
	return out, nil
}

// makeCover makes one cover with a request under run, and counts what it
// used. An error that stops the batch cancels run through stop. A made image
// is kept and shown as a preview, which waits for a slot under ctx, not run:
// an image already paid for is not dropped because the call stopped.
func (a *CoverArt) makeCover(ctx, run context.Context, stop context.CancelCauseFunc, j coverJob) NewCover {
	c := NewCover{ItemID: j.itemID, Title: j.title, Style: j.style}
	res, err := a.model.GenerateImage(run, j.req)
	a.count(j.req.Model, res.Usage, err == nil)
	if err != nil {
		if status, ok := errors.AsType[*gemini.StatusError](err); ok && status.StopsBatch() {
			stop(status)
		}
		c.Problem = notMadeProblem(err, run.Err() != nil)
		return c
	}
	img := abs.Image(res.Image)
	shown, err := a.previewWhenFree(ctx, img)
	if err != nil {
		c.Problem = problemOf(err) // made and paid for, but not to be set unseen
		return c
	}
	c.ImageID, c.Preview = a.keep(j.itemID, img), &shown
	return c
}

// notMadeProblem is the phrase that says why a cover was not made, given the
// error of its request and whether the call had stopped. Only a refusal's
// reason and an error's status name come from Google, and only when they are
// among Google's codes.
func notMadeProblem(err error, stopped bool) string {
	if blocked, ok := errors.AsType[*gemini.BlockedError](err); ok {
		if isCode(blocked.Reason) {
			return "Google would not make this image (" + blocked.Reason + ")"
		}
		return "Google would not make this image"
	}
	status, isStatus := errors.AsType[*gemini.StatusError](err)
	switch {
	case isStatus && status.StopsBatch():
		return problemStopped
	case isStatus && isCode(status.Status):
		return fmt.Sprintf("Gemini error (HTTP %d %s)", status.Code, status.Status)
	case isStatus:
		return fmt.Sprintf("Gemini error (HTTP %d)", status.Code)
	case stopped:
		return problemStopped
	case isTimeout(err):
		return problemTimedOut
	}
	return problemNotMade
}

// stoppedMessage says why a call stopped: Google answered status, an error
// that every further request would get too. Google's status name and message
// are kept only as plain text, and the name only when it is one of Google's
// codes.
func stoppedMessage(status *gemini.StatusError) string {
	code := fmt.Sprintf("HTTP %d", status.Code)
	if isCode(status.Status) {
		code += " " + status.Status
	}
	said := ""
	if msg := strings.TrimSuffix(plainText(status.Message), "."); msg != "" {
		said = ": " + msg
	}
	why := "This is a spending limit, a quota or used-up prepaid credit"
	if status.Code != http.StatusPaymentRequired && status.Code != http.StatusTooManyRequests {
		why = "The Gemini API key does not work or lacks a permission: check it with audiobookshelf-mcp setup"
	}
	return fmt.Sprintf("Google refused more images (%s)%s. %s; nothing is retried. Covers already made can still be set.", code, said, why)
}

// isCode reports whether s looks like one of Google's codes, such as
// "IMAGE_SAFETY": capital letters, digits and underscores, at most 64 of them.
func isCode(s string) bool {
	return s != "" && len(s) <= 64 && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") == ""
}

// isNotFound reports whether err is Audiobookshelf's 404.
func isNotFound(err error) bool {
	status, ok := errors.AsType[*abs.StatusError](err)
	return ok && status.StatusCode == http.StatusNotFound
}

// previewWhenFree makes the [preview] of img once it has a slot, which it gives
// back when done. It gives up with the error of ctx if that ends first.
func (a *CoverArt) previewWhenFree(ctx context.Context, img abs.Image) (abs.Image, error) {
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	case <-ctx.Done():
		return abs.Image{}, ctx.Err()
	}
	return a.preview(img)
}

// count adds what a request to model used to the spend, and an image if it made
// one.
func (a *CoverArt) count(model gemini.Model, u gemini.Usage, made bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if made {
		a.made++
	}
	used := a.used[model]
	used.PromptTokens += u.PromptTokens
	used.OutputTokens += u.OutputTokens
	used.ThoughtTokens += u.ThoughtTokens
	a.used[model] = used
}

// spend is the spend so far. The cost is worked out from the tokens counted,
// each model's at its own prices and the models in a fixed order, so it adds up
// without rounding errors piling up.
func (a *CoverArt) spend() Spend {
	a.mu.Lock()
	defer a.mu.Unlock()
	var usd float64
	for _, model := range slices.Sorted(maps.Keys(a.used)) {
		u, p := a.used[model], modelPrices[model]
		usd += float64(u.PromptTokens)*p.prompt + float64(u.OutputTokens)*p.output + float64(u.ThoughtTokens)*p.thought
	}
	return Spend{Images: a.made, EstimatedUSD: usd}
}

// keep keeps img, made for the book itemID, and returns its new ID: "gen_" and
// 16 random hex digits. The oldest image goes when [keptImages] are kept.
func (a *CoverArt) keep(itemID string, img abs.Image) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // never fails: it crashes the program instead
	id := "gen_" + hex.EncodeToString(b)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropExpired()
	if over := len(a.kept) - keptImages + 1; over > 0 {
		a.kept = slices.Delete(a.kept, 0, over)
	}
	a.kept = append(a.kept, keptImage{id: id, itemID: itemID, image: img, made: a.now()})
	return id
}

// lookup finds the kept image id. One never kept, or let go, is an
// [ErrUnknownImage] error.
func (a *CoverArt) lookup(id string) (keptImage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dropExpired()
	i := slices.IndexFunc(a.kept, func(k keptImage) bool { return k.id == id })
	if i < 0 {
		return keptImage{}, ErrUnknownImage
	}
	return a.kept[i], nil
}

// dropExpired lets go of the images kept for [keepImagesFor] or longer. The
// caller holds a.mu.
func (a *CoverArt) dropExpired() {
	now := a.now()
	a.kept = slices.DeleteFunc(a.kept, func(k keptImage) bool { return now.Sub(k.made) >= keepImagesFor })
}
