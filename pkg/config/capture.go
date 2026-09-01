package config

import (
	"fmt"
	"time"

	"github.com/urfave/cli/v3"
)

type CaptureConfig struct {
	SemanticEnabled bool
	Timeout         time.Duration
	MaxJSONBytes    int
}

func CaptureFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:    "capture.semantic-enabled",
			Value:   false,
			Usage:   "Enable opt-in semantic panel capture. [config: capture.semantic-enabled]",
			Sources: FromConfig("capture.semantic-enabled", "CAPTURE_SEMANTIC_ENABLED"),
		},
		&cli.DurationFlag{
			Name:    "capture.timeout",
			Value:   5 * time.Second,
			Usage:   "Maximum collector time. [config: capture.timeout]",
			Sources: FromConfig("capture.timeout", "CAPTURE_TIMEOUT"),
			Validator: func(v time.Duration) error {
				if v <= 0 {
					return fmt.Errorf("capture timeout must be positive")
				}
				return nil
			},
		},
		&cli.IntFlag{
			Name:    "capture.max-json-bytes",
			Value:   1048576,
			Usage:   "Maximum serialized semantic JSON body. [config: capture.max-json-bytes]",
			Sources: FromConfig("capture.max-json-bytes", "CAPTURE_MAX_JSON_BYTES"),
			Validator: func(v int) error {
				if v <= 0 {
					return fmt.Errorf("capture max-json-bytes must be positive")
				}
				return nil
			},
		},
	}
}

func CaptureConfigFromCommand(c *cli.Command) (CaptureConfig, error) {
	return CaptureConfig{
		SemanticEnabled: c.Bool("capture.semantic-enabled"),
		Timeout:         c.Duration("capture.timeout"),
		MaxJSONBytes:    c.Int("capture.max-json-bytes"),
	}, nil
}
