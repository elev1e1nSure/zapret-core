package main

import (
	"context"
	"strings"
	"time"
)

// Optimizer finds working DPI bypass strategies using a 2-phase approach:
//
//	Phase 1 probes each DesyncMethod family with default params to identify
//	methods that improve on the baseline (no-winws) score.
//	Phase 2 sweeps Fooling × TLSMode combinations only for the alive methods.
//
// This avoids testing 70+ candidates when none of them work on a given ISP:
// if Phase 1 finds no improvement, we bail out after ~14 tests (~84s) instead
// of spending 7+ minutes exhausting the full search space.
type Optimizer struct {
	asn          string
	kb           *Knowledge
	progressChan chan FindProgress
	ctx          context.Context
}

// NewOptimizer creates an optimizer without progress reporting.
func NewOptimizer(asn string, kb *Knowledge) *Optimizer {
	return NewOptimizerWithProgress(asn, kb, nil, context.Background())
}

// NewOptimizerWithProgress creates an optimizer with SSE progress reporting
// and context-based cancellation. Pass context.Background() if not needed.
func NewOptimizerWithProgress(asn string, kb *Knowledge, progressChan chan FindProgress, ctx context.Context) *Optimizer {
	return &Optimizer{
		asn:          asn,
		kb:           kb,
		progressChan: progressChan,
		ctx:          ctx,
	}
}

// Run executes the 2-phase search and returns the winning strategy + vector,
// or nil/zero if nothing works.
func (o *Optimizer) Run() (*Strategy, StrategyVector) {
	// --- Baseline: measure connectivity without winws ---
	StopWinws()
	time.Sleep(500 * time.Millisecond)
	baseline, _ := checkTargets()
	logInfo("[optimizer] baseline score=%.2f (no winws)", baseline)

	if baseline >= Cfg.ScoreThreshold {
		logInfo("[optimizer] targets already accessible, no bypass needed")
		return nil, StrategyVector{}
	}

	// --- Known-good strategies: fast path before full search ---
	known := o.kb.BestForASN(o.asn, 10)
	for i, v := range known {
		if err := o.checkCancel(); err != nil {
			return nil, StrategyVector{}
		}
		s := VectorToStrategy(v, i+1)
		logInfo("[kb] %d/%d testing known strategy: %s", i+1, len(known), s.Name)
		result := TestStrategy(s)
		if result.Score >= Cfg.ScoreThreshold {
			logInfo("[kb] known strategy works (score=%.2f)", result.Score)
			StartWinws(s)
			return s, v
		}
	}

	// --- Phase 1: probe each DesyncMethod family with default params ---
	methods := SearchSpace.DesyncMethod
	logInfo("[phase1] probing %d method families", len(methods))

	aliveMethods := []string{}
	for i, method := range methods {
		if err := o.checkCancel(); err != nil {
			return nil, StrategyVector{}
		}

		v := defaultVector(method)
		s := VectorToStrategy(v, i+1)
		logInfo("[phase1] %d/%d testing: %s", i+1, len(methods), s.Name)
		o.sendProgress(i+1, len(methods), s.Name, 0)

		result := TestStrategy(s)
		o.sendProgress(i+1, len(methods), s.Name, result.Score)

		if result.Score >= Cfg.ScoreThreshold {
			logInfo("[phase1] winner found (score=%.2f)", result.Score)
			StartWinws(s)
			return s, v
		}
		if result.Score > baseline {
			aliveMethods = append(aliveMethods, method)
			logInfo("[phase1] method %s alive (score=%.2f)", method, result.Score)
		}
	}

	if len(aliveMethods) == 0 {
		logWarn("[optimizer] phase1 found no promising methods — giving up")
		return nil, StrategyVector{}
	}
	logInfo("[phase1] %d methods alive: %v", len(aliveMethods), aliveMethods)

	// --- Phase 2: parameter sweep for alive methods only ---
	candidates := generateCandidatesFor(aliveMethods)
	logInfo("[phase2] %d candidates for %d alive methods", len(candidates), len(aliveMethods))

	phase2Base := len(methods)
	phase2Total := phase2Base + len(candidates)

	for i, v := range candidates {
		if err := o.checkCancel(); err != nil {
			return nil, StrategyVector{}
		}

		s := VectorToStrategy(v, phase2Base+i+1)
		logInfo("[phase2] %d/%d testing: %s", i+1, len(candidates), s.Name)
		o.sendProgress(phase2Base+i+1, phase2Total, s.Name, 0)

		result := TestStrategy(s)
		o.sendProgress(phase2Base+i+1, phase2Total, s.Name, result.Score)

		if result.Score >= Cfg.ScoreThreshold {
			logInfo("[phase2] winner found (score=%.2f)", result.Score)
			StartWinws(s)
			return s, v
		}
		logWarn("Not good enough (score=%.2f)", result.Score)
	}

	return nil, StrategyVector{}
}

// checkCancel returns an error if the context is done, nil otherwise.
func (o *Optimizer) checkCancel() error {
	select {
	case <-o.ctx.Done():
		logInfo("[optimizer] search cancelled")
		return o.ctx.Err()
	default:
		return nil
	}
}

// sendProgress sends a progress update if the channel is set.
func (o *Optimizer) sendProgress(current, total int, strategy string, score float64) {
	if o.progressChan != nil {
		o.progressChan <- FindProgress{
			Current:  current,
			Total:    total,
			Strategy: strategy,
			Score:    score,
		}
	}
}

// generateCandidatesFor produces strategy vectors for the given methods only,
// covering all Fooling × TLSMode combinations, deduplicated by actual winws args.
func generateCandidatesFor(methods []string) []StrategyVector {
	allowed := make(map[string]bool, len(methods))
	for _, m := range methods {
		allowed[m] = true
	}

	ss := SearchSpace
	raw := make([]StrategyVector, 0, len(methods)*len(ss.Fooling)*len(ss.TLSMode))
	for _, method := range ss.DesyncMethod {
		if !allowed[method] {
			continue
		}
		for _, fooling := range ss.Fooling {
			for _, tlsMode := range ss.TLSMode {
				v := defaultVector(method)
				v.Fooling = fooling
				v.TLSMode = tlsMode
				raw = append(raw, v)
			}
		}
	}
	return deduplicateVectors(raw)
}

// deduplicateVectors removes vectors that produce identical winws command lines.
func deduplicateVectors(raw []StrategyVector) []StrategyVector {
	seen := make(map[string]bool, len(raw))
	out := make([]StrategyVector, 0, len(raw))
	for _, v := range raw {
		key := strings.Join(Generate(v), "|")
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}

// defaultVector returns a StrategyVector with sensible fixed defaults for the given method.
// The caller overwrites the axes being varied.
func defaultVector(method string) StrategyVector {
	ss := SearchSpace
	return StrategyVector{
		DesyncMethod:    method,
		RepeatsTCP:      8,
		RepeatsUDP:      6,
		SplitPos:        "1",
		TLSFiles:        ss.TLSFiles[0],
		TLSMod:          ss.TLSMod[0],
		SeqOvl:          defaultSeqOvl(method),
		SeqOvlPattern:   ss.SeqOvlPattern[0],
		HostFakeMod:     ss.HostFakeMod[0],
		Cutoff:          "n3",
		BadseqIncrement: 2,
		QuicBin:         ss.QuicBin[0],
		AnyProtocol:     true,
		IPID:            "zero",
	}
}

// defaultSeqOvl returns a sensible seqovl default based on desync method.
func defaultSeqOvl(method string) int {
	if containsStr(method, "multisplit") {
		return 664
	}
	return 0
}
