package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Ejupi-Labs/council/v2/internal/domain"
	"github.com/Ejupi-Labs/council/v2/internal/evidence"
	"github.com/Ejupi-Labs/council/v2/internal/localstate"
	"github.com/Ejupi-Labs/council/v2/internal/project"
)

// GraphNamespaceProbeRequest names the actual allocated assembly workspace.
// WorkspaceIdentity is private controller state, never a public graph binding.
type GraphNamespaceProbeRequest struct {
	OperationAttemptID string
	Snapshot           *Snapshot
	WorkspaceIdentity  string
}

// GraphNamespaceProbeResult observes ASCII case handling in one exact directory.
// Full native materialization still validates actual names. A capability is
// usable only after complete acknowledged cleanup and evidence.
type GraphNamespaceProbeResult struct {
	CaseSensitive         *bool
	Scope                 string
	EffectsArtifact       *domain.GraphArtifactRef
	EffectCount           string
	EffectsKnown          bool
	EvidenceStatus, State string
	WorkspaceIdentity     string
}

type graphNamespaceProbeEffects struct {
	Contract           string `json:"contract"`
	OperationAttemptID string `json:"operationAttemptId"`
	WorkspaceID        string `json:"workspaceId"`
	Scope              string `json:"scope"`
	LowerName          string `json:"lowerName"`
	UpperName          string `json:"upperName"`
	LowerCreated       bool   `json:"lowerCreated"`
	UpperCreated       bool   `json:"upperCreated"`
	LowerRemoved       bool   `json:"lowerRemoved"`
	UpperRemoved       bool   `json:"upperRemoved"`
	CaseSensitive      *bool  `json:"caseSensitive"`
	EffectCount        string `json:"effectCount"`
	EffectsKnown       bool   `json:"effectsKnown"`
	EvidenceStatus     string `json:"evidenceStatus"`
	State              string `json:"state"`
}

type graphNamespaceSentinel struct {
	name    string
	file    *os.File
	info    os.FileInfo
	created bool
}

// ProbeGraphNamespace performs no Git commands, project writes or index changes.
// Each counted effect is an observed sentinel creation or verified name removal.
func (m *Manager) ProbeGraphNamespace(ctx context.Context, request GraphNamespaceProbeRequest) (GraphNamespaceProbeResult, error) {
	const scope = "ascii_case_in_exact_directory"
	result := GraphNamespaceProbeResult{Scope: scope, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "complete", State: "blocked"}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	source := request.Snapshot
	if !validManagedID(request.OperationAttemptID) || source == nil || source.AttemptID != request.OperationAttemptID || request.WorkspaceIdentity == "" {
		return result, ErrBlocked
	}
	if err := project.ValidateIdentity(source.WorkspacePath, request.WorkspaceIdentity); err != nil {
		return result, err
	}
	root, err := m.graphIndexTarget(source)
	if err != nil {
		return result, err
	}
	directory, err := root.Open(".")
	if err != nil {
		return result, errors.Join(err, root.Close())
	}
	closeTarget := func() error {
		var closeErr error
		if directory != nil {
			closeErr = directory.Close()
			directory = nil
		}
		if root != nil {
			closeErr = errors.Join(closeErr, root.Close())
			root = nil
		}
		return closeErr
	}
	held, err := directory.Stat()
	if err != nil || !held.IsDir() {
		return result, errors.Join(ErrBlocked, err, closeTarget())
	}
	if err = localstate.VerifyPrivateFile(directory, true); err != nil {
		return result, errors.Join(err, closeTarget())
	}
	checkTarget := func() error {
		if err := m.lock.CheckHeld(); err != nil {
			return err
		}
		if err := project.ValidateIdentity(source.WorkspacePath, request.WorkspaceIdentity); err != nil {
			return err
		}
		pathInfo, err := m.root.Lstat("workspaces/" + source.ID)
		if err != nil || !pathInfo.IsDir() || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(held, pathInfo) {
			return errors.Join(ErrBlocked, err)
		}
		current, err := directory.Stat()
		if err != nil || !os.SameFile(held, current) {
			return errors.Join(ErrBlocked, err)
		}
		controlInfo, err := root.Lstat(".git")
		if err != nil || !controlInfo.Mode().IsRegular() || controlInfo.Size() > 4096 {
			return errors.Join(ErrBlocked, err)
		}
		control, err := root.ReadFile(".git")
		if err != nil || hashBytes(control) != source.GitControlSHA256 {
			return errors.Join(ErrBlocked, err)
		}
		return nil
	}
	if err = checkTarget(); err != nil {
		return result, errors.Join(err, closeTarget())
	}
	id, err := evidence.NewID()
	if err != nil {
		return result, errors.Join(err, closeTarget())
	}
	lower := graphNamespaceSentinel{name: ".council-namespace-" + id + "-a"}
	upper := graphNamespaceSentinel{name: strings.ToUpper(lower.name)}
	for _, name := range []string{lower.name, upper.name} {
		if _, err = root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return result, errors.Join(ErrBlocked, err, closeTarget())
		}
	}
	record := graphNamespaceProbeEffects{Contract: "006-namespace-probe-v1", OperationAttemptID: request.OperationAttemptID, WorkspaceID: source.ID,
		Scope: scope, LowerName: lower.name, UpperName: upper.name, EffectCount: "0", EffectsKnown: true, EvidenceStatus: "incomplete", State: "intent"}
	intent, err := m.graphRetainJSON(ctx, request.OperationAttemptID, "graph_operation_effects", record)
	if err != nil {
		result.EffectsKnown, result.EvidenceStatus, result.State = false, "uncertain", "uncertain"
		return result, errors.Join(err, closeTarget())
	}
	result.EffectsArtifact = &intent
	count, uncertain := 0, false
	body := []byte("Council private graph namespace probe (006-v1).\n")
	create := func(sentinel *graphNamespaceSentinel) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := checkTarget(); err != nil {
			return err
		}
		// An opaque failed create cannot establish whether a name was created.
		uncertain = true
		file, err := root.OpenFile(sentinel.name, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		sentinel.file, sentinel.created = file, true
		count++
		sentinel.info, err = file.Stat()
		if err != nil || !sentinel.info.Mode().IsRegular() {
			return errors.Join(ErrBlocked, err)
		}
		uncertain = false
		if err = localstate.ProtectPrivateFile(file, false); err != nil {
			return err
		}
		n, writeErr := file.Write(body)
		if writeErr == nil && n != len(body) {
			writeErr = io.ErrShortWrite
		}
		return errors.Join(writeErr, file.Sync())
	}
	var observation *bool
	operationErr := func() error {
		if err := create(&lower); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		alias, err := root.Open(upper.name)
		if err == nil {
			pathInfo, pathErr := root.Lstat(upper.name)
			info, statErr := alias.Stat()
			actual, readErr := io.ReadAll(io.LimitReader(alias, int64(len(body))+1))
			privateErr := localstate.VerifyPrivateFile(alias, false)
			closeErr := alias.Close()
			if pathErr != nil || statErr != nil || readErr != nil || privateErr != nil || closeErr != nil ||
				!pathInfo.Mode().IsRegular() || !info.Mode().IsRegular() || !os.SameFile(pathInfo, info) ||
				!os.SameFile(lower.info, info) || !bytes.Equal(actual, body) {
				if closeErr != nil {
					uncertain = true
				}
				return errors.Join(ErrBlocked, pathErr, statErr, readErr, privateErr, closeErr)
			}
			value := false
			observation = &value
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := create(&upper); err != nil {
			return err
		}
		if os.SameFile(lower.info, upper.info) {
			return ErrBlocked
		}
		value := true
		observation = &value
		return nil
	}()

	// Cleanup is bounded controller settlement work, independent of caller
	// cancellation. No preexisting or identity-mismatched name is removed.
	closeSentinel := func(sentinel *graphNamespaceSentinel) error {
		if sentinel.file == nil {
			return nil
		}
		err := sentinel.file.Close()
		sentinel.file = nil
		return err
	}
	cleanupErr := errors.Join(closeSentinel(&upper), closeSentinel(&lower))
	if cleanupErr != nil {
		uncertain = true
	}
	remove := func(sentinel *graphNamespaceSentinel) error {
		if !sentinel.created {
			return nil
		}
		info, err := root.Lstat(sentinel.name)
		if err != nil || sentinel.info == nil || !info.Mode().IsRegular() || !os.SameFile(sentinel.info, info) {
			return errors.Join(ErrBlocked, err)
		}
		if err = root.Remove(sentinel.name); err != nil {
			return err
		}
		count++
		return nil
	}
	cleanupErr = errors.Join(cleanupErr, remove(&upper), remove(&lower))
	absent := func(name string) (bool, error) {
		_, err := root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, errors.Join(ErrBlocked, err)
	}
	record.LowerRemoved, err = absent(lower.name)
	cleanupErr = errors.Join(cleanupErr, err)
	record.UpperRemoved, err = absent(upper.name)
	cleanupErr = errors.Join(cleanupErr, err)
	if cleanupErr != nil {
		uncertain = true
	}
	stableErr := checkTarget()
	if stableErr == nil {
		stableErr = localstate.SyncPrivateDirectory(source.WorkspacePath)
	}
	if stableErr == nil {
		stableErr = checkTarget()
	}
	closeErr := closeTarget()
	if stableErr != nil || closeErr != nil {
		uncertain = true
	}
	operationErr = errors.Join(operationErr, cleanupErr, stableErr, closeErr)
	result.EffectCount = strconv.Itoa(count)
	result.EffectsKnown = !uncertain
	result.State, result.EvidenceStatus = "incomplete", "incomplete"
	if uncertain {
		result.State, result.EvidenceStatus = "uncertain", "uncertain"
	} else if operationErr == nil && observation != nil {
		result.State, result.EvidenceStatus = "complete", "complete"
		record.CaseSensitive = observation
	}
	record.LowerCreated, record.UpperCreated = lower.created, upper.created
	record.State, record.EvidenceStatus, record.EffectsKnown, record.EffectCount = result.State, result.EvidenceStatus, result.EffectsKnown, result.EffectCount
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	terminal, saveErr := m.graphRetainJSON(settlement, request.OperationAttemptID, "graph_operation_effects", record)
	if saveErr != nil {
		result.EffectsKnown, result.EvidenceStatus, result.State = false, "uncertain", "uncertain"
		return result, errors.Join(operationErr, saveErr)
	}
	result.EffectsArtifact = &terminal
	if operationErr != nil {
		return result, operationErr
	}
	if result.State != "complete" || observation == nil {
		return result, ErrBlocked
	}
	result.CaseSensitive, result.WorkspaceIdentity = observation, request.WorkspaceIdentity
	return result, nil
}
