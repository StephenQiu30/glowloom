package application

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAudioDurationRetainsBoundedJSONExponentExactly(t *testing.T) {
	for _, text := range []string{"6e1", "1e-19"} {
		value, err := ParseAudioDurationSeconds(json.RawMessage(text), 120)
		require.NoError(t, err)
		require.Equal(t, json.Number(text), value)
	}
	for _, text := range []string{"0", "null", `"60"`, "-1", "1e99999999999999999999", "1e513", "1e-513"} {
		_, err := ParseAudioDurationSeconds(json.RawMessage(text), 120)
		require.Error(t, err, text)
	}
}
