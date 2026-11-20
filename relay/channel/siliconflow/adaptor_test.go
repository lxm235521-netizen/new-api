package siliconflow

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestFIMAddsUserMessageWhenOnlySystemPromptPresent(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Prefix: "prefix",
		Suffix: "suffix",
		Messages: []dto.Message{
			{Role: "system", Content: "channel prompt"},
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, req)
	require.NoError(t, err)

	convertedReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, convertedReq.Messages, 2)
	require.Equal(t, "system", convertedReq.Messages[0].Role)
	require.Equal(t, "channel prompt", convertedReq.Messages[0].StringContent())
	require.Equal(t, "user", convertedReq.Messages[1].Role)
	require.Equal(t, "", convertedReq.Messages[1].StringContent())
}

func TestConvertOpenAIRequestFIMKeepsExistingUserMessage(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Prefix: "prefix",
		Suffix: "suffix",
		Messages: []dto.Message{
			{Role: "user", Content: "original"},
		},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, req)
	require.NoError(t, err)

	convertedReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, convertedReq.Messages, 1)
	require.Equal(t, "original", convertedReq.Messages[0].StringContent())
}
