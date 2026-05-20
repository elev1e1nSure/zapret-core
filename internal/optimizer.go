package main

import (
	"math"
	"math/rand"
)

// arm represents a single strategy candidate with UCB1 statistics
type arm struct {
	vector     StrategyVector
	totalScore float64
	pulls      int
}

// ucb1Score returns the UCB1 value for this arm
// Balances exploitation (known good) vs exploration (untested)
func (a *arm) ucb1Score(totalPulls int) float64 {
	if a.pulls == 0 {
		return math.Inf(1) // always test untried arms first
	}
	avgScore := a.totalScore / float64(a.pulls)
	exploration := math.Sqrt(2 * math.Log(float64(totalPulls)) / float64(a.pulls))
	return avgScore + exploration
}

// Optimizer runs UCB1 algorithm over the strategy search space to find working DPI bypass strategies
type Optimizer struct {
	asn          string
	kb           *Knowledge
	arms         []*arm
	totalPulls   int
	progressChan chan FindProgress
}

// NewOptimizer builds the arm pool from SearchSpace + known good strategies for ASN
func NewOptimizer(asn string, kb *Knowledge) *Optimizer {
	return NewOptimizerWithProgress(asn, kb, nil)
}

// NewOptimizerWithProgress builds optimizer with progress callback channel
func NewOptimizerWithProgress(asn string, kb *Knowledge, progressChan chan FindProgress) *Optimizer {
	arms := buildArms(asn, kb)
	return &Optimizer{
		asn:          asn,
		kb:           kb,
		arms:         arms,
		progressChan: progressChan,
	}
}

// Run iterates until a working strategy is found or arms are exhausted.
// Returns the winning Strategy and its StrategyVector, or nil/zero on failure.
func (o *Optimizer) Run() (*Strategy, StrategyVector) {
	logInfo("Search space: %d candidate strategies", len(o.arms))

	for i := 0; i < len(o.arms); i++ {
		a := o.selectArm()
		if a == nil {
			break
		}

		strategy := VectorToStrategy(a.vector, i+1)
		logInfo("[%d/%d] Testing: %s", i+1, len(o.arms), strategy.Name)

		// Send progress update if channel is set
		if o.progressChan != nil {
			o.progressChan <- FindProgress{
				Current:  i + 1,
				Total:    len(o.arms),
				Strategy: strategy.Name,
				Score:    0.0,
			}
		}

		result := TestStrategy(strategy)
		o.totalPulls++
		a.pulls++
		a.totalScore += result.Score

		// Send progress update after test
		if o.progressChan != nil {
			o.progressChan <- FindProgress{
				Current:  i + 1,
				Total:    len(o.arms),
				Strategy: strategy.Name,
				Score:    result.Score,
			}
		}

		if result.Score >= Cfg.ScoreThreshold {
			logInfo("Strategy works (score=%.2f)", result.Score)
			StopWinws()
			StartWinws(strategy)
			return strategy, a.vector
		}

		logWarn("Not good enough (score=%.2f)", result.Score)
	}

	return nil, StrategyVector{}
}

// selectArm picks the arm with the highest UCB1 score
func (o *Optimizer) selectArm() *arm {
	best := -math.MaxFloat64
	var selected *arm

	for _, a := range o.arms {
		score := a.ucb1Score(o.totalPulls + 1)
		if score > best {
			best = score
			selected = a
		}
	}

	return selected
}

// buildArms creates the full arm pool:
// 1. Known good strategies for this ASN — pre-seeded so UCB1 picks them early
// 2. New candidates sampled from SearchSpace
func buildArms(asn string, kb *Knowledge) []*arm {
	known := kb.BestForASN(asn, 999)
	arms := make([]*arm, 0, len(known)+64)

	for _, v := range known {
		arms = append(arms, &arm{vector: v, totalScore: 1.0, pulls: 1})
	}
	for _, v := range generateCandidates() {
		arms = append(arms, &arm{vector: v})
	}
	return arms
}

// generateCandidates produces a structured set of vectors from SearchSpace.
// Covers all combinations of high-impact axes first (DesyncMethod × Fooling × TLSMode),
// then appends 64 random samples for long-tail exploration.
func generateCandidates() []StrategyVector {
	ss := SearchSpace
	vectors := []StrategyVector{}

	// High-impact grid
	for _, method := range ss.DesyncMethod {
		for _, fooling := range ss.Fooling {
			for _, tlsMode := range ss.TLSMode {
				v := defaultVector(method)
				v.Fooling = fooling
				v.TLSMode = tlsMode
				vectors = append(vectors, v)
			}
		}
	}

	// Random exploration
	for i := 0; i < 64; i++ {
		method := ss.DesyncMethod[rand.Intn(len(ss.DesyncMethod))]
		v := defaultVector(method)
		v.Fooling = ss.Fooling[rand.Intn(len(ss.Fooling))]
		v.TLSMode = ss.TLSMode[rand.Intn(len(ss.TLSMode))]
		v.RepeatsTCP = ss.RepeatsTCP[rand.Intn(len(ss.RepeatsTCP))]
		v.RepeatsUDP = ss.RepeatsUDP[rand.Intn(len(ss.RepeatsUDP))]
		v.SplitPos = ss.SplitPos[rand.Intn(len(ss.SplitPos))]
		v.TLSFiles = ss.TLSFiles[rand.Intn(len(ss.TLSFiles))]
		v.TLSMod = ss.TLSMod[rand.Intn(len(ss.TLSMod))]
		v.SeqOvl = ss.SeqOvl[rand.Intn(len(ss.SeqOvl))]
		v.SeqOvlPattern = ss.SeqOvlPattern[rand.Intn(len(ss.SeqOvlPattern))]
		v.HostFakeMod = ss.HostFakeMod[rand.Intn(len(ss.HostFakeMod))]
		v.Cutoff = ss.Cutoff[rand.Intn(len(ss.Cutoff))]
		v.BadseqIncrement = ss.BadseqIncrement[rand.Intn(len(ss.BadseqIncrement))]
		v.IPID = ss.IPID[rand.Intn(len(ss.IPID))]
		vectors = append(vectors, v)
	}

	return vectors
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

// defaultSeqOvl returns a sensible seqovl default based on desync method
func defaultSeqOvl(method string) int {
	if containsStr(method, "multisplit") {
		return 664
	}
	return 0
}
