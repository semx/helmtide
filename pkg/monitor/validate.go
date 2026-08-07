package monitor

func (c *config) Validate() error {
	if c.Name() == "" {
		return ErrNameEmpty
	}

	if c.TotalTimeout < c.IterationTimeout {
		return ErrLowTotalTimeout
	}

	if c.Interval <= 0 {
		return ErrLowInterval
	}

	if c.SuccessThreshold < 1 {
		return ErrLowSuccessThreshold
	}

	if c.FailureThreshold < 1 {
		return ErrLowFailureThreshold
	}

	err := c.subConfig.Validate()
	if err != nil {
		return NewSubMonitorError(err)
	}

	return nil
}
