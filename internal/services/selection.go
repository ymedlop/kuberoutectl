package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ymedlop/kuberoutectl/internal/cache"
	"github.com/ymedlop/kuberoutectl/internal/domain"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// SelectionService records the operator's current target or collection choice.
//
// "use" persists the selection so the CLI can show "what am I pointed at", and
// (optionally) materializes the target into the local kubeconfig through the
// provider's ContextActivator capability. Selection stays provider-agnostic;
// the kubeconfig side effect is delegated to the provider.
type SelectionService struct {
	store    cache.CacheStore
	registry *providers.Registry
	now      func() time.Time
}

// NewSelectionService builds a SelectionService. A nil now defaults to
// time.Now().UTC(). registry may be nil if activation is never requested.
func NewSelectionService(store cache.CacheStore, registry *providers.Registry, now func() time.Time) *SelectionService {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SelectionService{store: store, registry: registry, now: now}
}

// UseTargetOptions carries the caller's intent into UseTarget. The zero value
// records a selection without touching the kubeconfig and lets the service pick
// the access path.
type UseTargetOptions struct {
	// Refresh re-establishes operability against the provider instead of
	// trusting the last sync. Off by default: the cached verdict covers every
	// cluster, so a live check buys freshness rather than coverage and should
	// cost a call only when asked for.
	Refresh bool

	// Activate also fetches the target's credentials into the local kubeconfig.
	Activate bool
	// CredentialName selects which of the target's credentials to go in
	// through, matched against Credential.Name — the AWS adapter sets that to
	// the profile, so `--profile ops` is spelled here as CredentialName "ops"
	// without the core learning what a profile is. Empty means "decide for me".
	CredentialName string
}

// CredentialSource explains how UseTarget arrived at the credential it used, so
// callers can tell a decision from a guess. Landing on a target's primary
// carries no evidence that it is the right way in — for a cluster several
// profiles can reach, it is only the healthiest one — so the CLI must be able
// to say "default" rather than presenting it as a choice.
type CredentialSource int

const (
	// CredentialFromDefault: nothing was asked for, so the primary was used.
	CredentialFromDefault CredentialSource = iota
	// CredentialFromFlag: the caller named it.
	CredentialFromFlag
	// CredentialFromMemory: reused from the persisted selection.
	CredentialFromMemory
)

// String renders the source for display and for the wire. Both the CLI and the
// MCP server go through this, so the two surfaces cannot describe the same
// decision differently — and an iota int never reaches a JSON consumer, where
// it would mean nothing without reading this file.
func (s CredentialSource) String() string {
	switch s {
	case CredentialFromFlag:
		return "flag"
	case CredentialFromMemory:
		return "remembered"
	default:
		return "default"
	}
}

// MarshalJSON keeps the semantic name in any JSON rendering of this type.
func (s CredentialSource) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// UseTargetResult is what a selection resolved to.
type UseTargetResult struct {
	Target domain.Target `json:"target"`
	// Credential is the access path used. It is the zero value only when the
	// target names a credential the snapshot no longer holds.
	Credential       domain.Credential `json:"credential"`
	CredentialSource CredentialSource  `json:"credential_source"`
	// LostCredentialID names a credential this target was previously used
	// through that the cache no longer offers, so the caller fell back to the
	// default. Empty when nothing was lost.
	//
	// This is not derivable from the result alone: once the fold drops the
	// vanished credential, the target has one access path again and looks
	// exactly like a target that never had a choice. Without this field the
	// operator who deliberately picked a break-glass profile gets moved back to
	// the default with no output at all.
	LostCredentialID domain.CredentialID `json:"lost_credential_id,omitempty"`

	// AccessWarning is set only when the credential in use is *confirmed* to
	// hold no access entry on this target — never when operability is merely
	// unknown. Most clusters authenticate through the aws-auth ConfigMap, where
	// nothing can be established, so warning on unknown would fire constantly on
	// no evidence and train the operator to ignore the one case that matters.
	AccessWarning string `json:"access_warning,omitempty"`

	// AccessVerdict is what is known about the credential in use — operable, not
	// operable, or unknown. Reported in both directions, unlike AccessWarning:
	// the operator asked whether they can operate here, and silence answers only
	// half of that.
	AccessVerdict domain.AccessVerdict `json:"access_verdict,omitempty"`
	// AccessReason explains why a --refresh check could not conclude. Empty when
	// it did, and when none was asked for.
	AccessReason string `json:"access_reason,omitempty"`
}

// accessWarning renders the caution for a credential the target is known to
// refuse, naming a credential that would work when one exists.
//
// It reports rather than blocks: the verdict comes from the last sync and may
// be stale, and going into a cluster to diagnose exactly this is a legitimate
// thing to do.
func accessWarning(t domain.Target, used domain.Credential, reaching []domain.Credential, live bool) string {
	if t.CredentialAccess(used.ID) != domain.AccessNotOperable {
		return ""
	}
	var alternatives []string
	for _, c := range reaching {
		if t.CredentialAccess(c.ID) == domain.AccessOperable {
			alternatives = append(alternatives, c.Name)
		}
	}
	when := " at the last sync"
	if live {
		// Under --refresh the answer is current, and saying otherwise would send
		// the operator to re-sync in search of a fresher one that does not exist.
		when = ""
	}
	msg := used.Name + " has no access entry on this cluster" + when + "; kubectl may return Forbidden."
	if len(alternatives) > 0 {
		msg += " " + strings.Join(alternatives, ", ") + " did have one."
	}
	return msg
}

// UseTarget records a target selection after resolving ref (a full ID, alias,
// or name) to exactly one target. When opts.Activate is true it also fetches the
// target's credentials into the local kubeconfig via the owning provider
// (setting the current context). The selection is only recorded after a
// requested activation succeeds, so a failed kubeconfig fetch doesn't silently
// change "what am I pointed at".
//
// The credential is resolved before anything external runs, so an unusable
// choice fails without having touched the kubeconfig.
func (s *SelectionService) UseTarget(ctx context.Context, ref string, opts UseTargetOptions) (UseTargetResult, error) {
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return UseTargetResult{}, err
	}
	AssignAliases(snap.Targets)
	hidden, err := loadHiddenSet(s.store)
	if err != nil {
		return UseTargetResult{}, err
	}
	ApplyVisibility(snap.Targets, hidden)
	found, err := ResolveTargetRef(snap.Targets, ref)
	if err != nil {
		return UseTargetResult{}, err
	}

	prior, err := s.store.LoadSelection()
	if err != nil {
		return UseTargetResult{}, err
	}
	remembered := domain.CredentialID("")
	if prior.TargetID == found.ID {
		remembered = prior.CredentialID
	}
	cred, source, err := resolveCredential(found, snap.Credentials, opts.CredentialName, remembered)
	if err != nil {
		return UseTargetResult{}, err
	}
	// A loss is a credential that went away, not merely one that differs from
	// what was remembered. An explicit choice deviates from the remembered value
	// by definition — reporting that as a loss would raise a false alarm on the
	// ordinary act of switching profiles. Gate on the reason for the difference,
	// not on the difference existing.
	lost := domain.CredentialID("")
	if remembered != "" && cred.ID != remembered && source != CredentialFromFlag {
		lost = remembered
	}

	if opts.Activate {
		if err := s.activate(ctx, found, cred, source); err != nil {
			return UseTargetResult{}, err
		}
	}

	sel := domain.Selection{TargetID: found.ID, CredentialID: cred.ID, UpdatedAt: s.now()}
	if err := s.store.SaveSelection(sel); err != nil {
		return UseTargetResult{}, err
	}
	reaching := credentialsFor(found, snap.Credentials)
	accessReason := ""
	if opts.Refresh {
		// Overrides the cached verdict rather than merging with it: two sources
		// for one answer is how they drift apart.
		live := checkAccess(ctx, s.registry, found, reaching)
		if live.Mode != "" || live.Reason != "" {
			found.AccessCheck, found.OperableCredentialIDs = live.Mode, live.Operable
			accessReason = live.Reason
		}
	}

	return UseTargetResult{
		Target: found, Credential: cred, CredentialSource: source, LostCredentialID: lost,
		AccessWarning: accessWarning(found, cred, reaching, opts.Refresh),
		AccessVerdict: found.CredentialAccess(cred.ID),
		AccessReason:  accessReason,
	}, nil
}

// credentialsFor returns the credentials that can reach a target, primary
// first. A target with no CredentialIDs — every provider with one way in, and
// every snapshot written before the field existed — yields just its
// CredentialID, which is why that path needs no migration.
func credentialsFor(t domain.Target, all []domain.Credential) []domain.Credential {
	byID := make(map[domain.CredentialID]domain.Credential, len(all))
	for _, c := range all {
		byID[c.ID] = c
	}
	ids := t.CredentialIDs
	if len(ids) == 0 {
		ids = []domain.CredentialID{t.CredentialID}
	}
	out := make([]domain.Credential, 0, len(ids))
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// resolveCredential decides which access path to use: an explicit name, else
// the one remembered from a previous use of this target, else the primary.
//
// A named credential that cannot reach this target is an error rather than a
// fallback. Falling back would put the operator on a different identity than
// the one they asked for, which in a fleet whose profiles differ in privilege
// is the mistake most worth failing loudly on. A *remembered* one that has
// since disappeared is different — the operator did not ask for it now, so it
// degrades quietly to the primary.
func resolveCredential(t domain.Target, all []domain.Credential, name string, remembered domain.CredentialID) (domain.Credential, CredentialSource, error) {
	reachable := credentialsFor(t, all)

	if name != "" {
		for _, c := range reachable {
			if c.Name == name {
				return c, CredentialFromFlag, nil
			}
		}
		names := make([]string, 0, len(reachable))
		for _, c := range reachable {
			names = append(names, c.Name)
		}
		if len(names) == 0 {
			return domain.Credential{}, 0, fmt.Errorf(
				"target %q has no known credentials; run `kuberoutectl sync %s` first", t.Name, t.ProviderID)
		}
		return domain.Credential{}, 0, fmt.Errorf(
			"%q cannot reach target %q; available: %s", name, t.Name, strings.Join(names, ", "))
	}

	if remembered != "" {
		for _, c := range reachable {
			if c.ID == remembered {
				return c, CredentialFromMemory, nil
			}
		}
		// Dropped by a resync: fall through to the primary rather than failing.
	}

	if len(reachable) == 0 {
		return domain.Credential{}, CredentialFromDefault, nil
	}
	return reachable[0], CredentialFromDefault, nil
}

// activate materializes a target into the kubeconfig via its provider, gated on
// the CanSwitchContext capability and the ContextActivator interface.
//
// When the credential was explicitly chosen, the provider must implement
// CredentialActivator; otherwise the choice cannot be honoured and activating
// through the primary anyway would silently ignore what the operator asked for.
func (s *SelectionService) activate(ctx context.Context, target domain.Target, cred domain.Credential, source CredentialSource) error {
	if s.registry == nil {
		return fmt.Errorf("cannot update kubeconfig: no provider registry configured")
	}
	p, ok := s.registry.Get(target.ProviderID)
	if !ok {
		return fmt.Errorf("provider %q for target %q is not registered", target.ProviderID, target.ID)
	}
	if !p.Capabilities().CanSwitchContext {
		return fmt.Errorf("provider %q cannot write kubeconfig; re-run with --no-kubeconfig to record the selection only", target.ProviderID)
	}

	if byCred, ok := p.(providers.CredentialActivator); ok && cred.ID != "" {
		return byCred.ActivateAs(ctx, target, cred)
	}
	if source == CredentialFromFlag {
		return fmt.Errorf("provider %q cannot activate through a chosen credential; drop the flag to use %q",
			target.ProviderID, target.CredentialID)
	}

	activator, ok := p.(providers.ContextActivator)
	if !ok {
		return fmt.Errorf("provider %q declares CanSwitchContext but does not implement activation", target.ProviderID)
	}
	return activator.Activate(ctx, target)
}

// UseCollection records a collection selection.
func (s *SelectionService) UseCollection(id domain.CollectionID) error {
	return s.store.SaveSelection(domain.Selection{CollectionID: id, UpdatedAt: s.now()})
}

// Current returns the persisted selection.
func (s *SelectionService) Current() (domain.Selection, error) {
	return s.store.LoadSelection()
}

// SelectionStatus is the render-friendly answer to "what am I pointed at?".
// Target/Collection are resolved from the current cache and nil when the
// selection is empty — or stale, i.e. it references something a later resync
// removed. Stale is deliberately not an error: the selection is still shown so
// the operator can see what it *was* and pick again.
type SelectionStatus struct {
	Selection  domain.Selection   `json:"selection"`
	Target     *domain.Target     `json:"target,omitempty"`
	Collection *domain.Collection `json:"collection,omitempty"`
	// Credential is the access path the selection was activated through,
	// resolved against the current cache. Nil when the selection records none
	// (every selection written before this existed) or when a resync dropped it.
	Credential *domain.Credential `json:"credential,omitempty"`
	// CredentialMissing distinguishes those two cases: true means the selection
	// names a credential the cache no longer holds, so `current` can say the
	// profile is gone instead of quietly showing nothing.
	CredentialMissing bool `json:"credential_missing,omitempty"`
	// SyncedAt is when the cache was last written, so the caller can show how
	// fresh the health information is.
	SyncedAt time.Time `json:"synced_at"`
}

// Status resolves the persisted selection against the current snapshot.
func (s *SelectionService) Status() (SelectionStatus, error) {
	sel, err := s.store.LoadSelection()
	if err != nil {
		return SelectionStatus{}, err
	}
	snap, err := s.store.LoadSnapshot()
	if err != nil {
		return SelectionStatus{}, err
	}
	st := SelectionStatus{Selection: sel, SyncedAt: snap.SyncedAt}

	if sel.TargetID != "" {
		AssignAliases(snap.Targets)
		hidden, err := loadHiddenSet(s.store)
		if err != nil {
			return SelectionStatus{}, err
		}
		ApplyVisibility(snap.Targets, hidden)
		for i := range snap.Targets {
			if snap.Targets[i].ID == sel.TargetID {
				t := snap.Targets[i]
				st.Target = &t
				break
			}
		}
	}
	if sel.CredentialID != "" {
		st.CredentialMissing = true
		for i := range snap.Credentials {
			if snap.Credentials[i].ID == sel.CredentialID {
				c := snap.Credentials[i]
				st.Credential, st.CredentialMissing = &c, false
				break
			}
		}
	}
	if sel.CollectionID != "" {
		cols, err := s.store.LoadCollections()
		if err != nil {
			return SelectionStatus{}, err
		}
		for i := range cols {
			if cols[i].ID == sel.CollectionID {
				c := cols[i]
				st.Collection = &c
				break
			}
		}
	}
	return st, nil
}
