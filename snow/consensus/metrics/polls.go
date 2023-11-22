// Copyright (C) 2019-2023, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ava-labs/avalanchego/utils"
)

var _ Polls = (*polls)(nil)

// Polls reports commonly used consensus poll metrics.
type Polls interface {
	Successful()
	Failed()
}

type polls struct {
	// numFailedPolls keeps track of the number of polls that failed
	numFailedPolls prometheus.Counter

	// numSuccessfulPolls keeps track of the number of polls that succeeded
	numSuccessfulPolls prometheus.Counter
}

func NewPolls(namespace string, reg prometheus.Registerer) (Polls, error) {
	p := &polls{
		numSuccessfulPolls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "polls_successful",
			Help:      "Number of successful polls",
		}),
		numFailedPolls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "polls_failed",
			Help:      "Number of failed polls",
		}),
	}
	err := utils.Err(
		reg.Register(p.numFailedPolls),
		reg.Register(p.numSuccessfulPolls),
	)
	return p, err
}

func (p *polls) Failed() {
	p.numFailedPolls.Inc()
}

func (p *polls) Successful() {
	p.numSuccessfulPolls.Inc()
}