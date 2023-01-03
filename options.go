package health

import "fmt"

type Option func(*Health) error

// WithChecks adds checks to newly instantiated health-container
func WithChecks(checks ...Check) Option {
	return func(h *Health) error {
		for _, c := range checks {
			if err := h.Register(c); err != nil {
				return fmt.Errorf("could not register check %q: %w", c.Name, err)
			}
		}

		return nil
	}
}

// WithComponent sets the component description of the component to which this check refer
func WithComponent(component Component) Option {
	return func(h *Health) error {
		h.component = component

		return nil
	}
}

// WithMaxConcurrent sets max number of concurrently running checks.
// Set to 1 if want to run all checks sequentially.
func WithMaxConcurrent(n int) Option {
	return func(h *Health) error {
		h.maxConcurrent = n
		return nil
	}
}

// WithSystemInfo enables the option to return system information about the go process.
func WithSystemInfo() Option {
	return func(h *Health) error {
		h.systemInfo = true
		return nil
	}
}
