package main

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse runs the CLI argument parser over args against a fresh options struct.
func parse(args []string) (options, error) {
	var o options
	p := flags.NewParser(&o, flags.None)
	_, err := p.ParseArgs(args)
	return o, err
}

func TestFlags_Defaults(t *testing.T) {
	o, err := parse([]string{"https://youtu.be/x"})
	require.NoError(t, err)
	assert.Equal(t, "https://youtu.be/x", o.Args.URL)
	assert.Equal(t, ".", o.Output)
	assert.Equal(t, "gpt-5-mini", o.Model)
	assert.Equal(t, "summary", o.Mode)
	assert.Equal(t, 0, o.MaxChunks)
}

func TestFlags_AllOverrides(t *testing.T) {
	o, err := parse([]string{
		"-o", "out", "-m", "gpt-5", "--mode", "lecture", "--max-chunks", "3",
		"https://youtu.be/x",
	})
	require.NoError(t, err)
	assert.Equal(t, "out", o.Output)
	assert.Equal(t, "gpt-5", o.Model)
	assert.Equal(t, "lecture", o.Mode)
	assert.Equal(t, 3, o.MaxChunks)
}

func TestFlags_LongOutput(t *testing.T) {
	o, err := parse([]string{"--output", "dir", "https://youtu.be/x"})
	require.NoError(t, err)
	assert.Equal(t, "dir", o.Output)
}

func TestFlags_InvalidModeRejected(t *testing.T) {
	_, err := parse([]string{"--mode", "bogus", "https://youtu.be/x"})
	require.Error(t, err)
}

func TestFlags_MissingURLRejected(t *testing.T) {
	_, err := parse([]string{})
	require.Error(t, err)
}
