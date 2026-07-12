package embed

import (
	"context"
	"fmt"
	"math"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Model constants for the default embedder (ADR-004).
const (
	ModelName    = "all-MiniLM-L6-v2"
	ModelDims    = 384
	maxSeqLength = 256 // sentence-transformers max_seq_length for this model
)

// ONNX runs all-MiniLM-L6-v2 in-process on CPU via ONNX Runtime.
// Pipeline (must match the sentence-transformers reference): tokenize →
// forward → mean pooling with attention mask → L2 normalize.
type ONNX struct {
	tok     *Tokenizer
	session *ort.DynamicAdvancedSession
}

// ONNXConfig locates the three files installed by `mnemo init`.
type ONNXConfig struct {
	ModelPath   string // model.onnx
	VocabPath   string // vocab.txt
	LibraryPath string // ONNX Runtime shared library
}

// The ONNX Runtime environment is process-global: first configuration wins.
var (
	ortOnce sync.Once
	ortErr  error
)

func initRuntime(libraryPath string) error {
	ortOnce.Do(func() {
		ort.SetSharedLibraryPath(libraryPath)
		ortErr = ort.InitializeEnvironment()
	})
	return ortErr
}

// NewONNX loads the tokenizer, the ONNX Runtime library, and the model.
func NewONNX(cfg ONNXConfig) (*ONNX, error) {
	tok, err := NewTokenizer(cfg.VocabPath)
	if err != nil {
		return nil, err
	}
	if err := initRuntime(cfg.LibraryPath); err != nil {
		return nil, fmt.Errorf("embed: initialize onnxruntime (%s): %w", cfg.LibraryPath, err)
	}
	session, err := ort.NewDynamicAdvancedSession(cfg.ModelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"}, nil)
	if err != nil {
		return nil, fmt.Errorf("embed: load model (%s): %w", cfg.ModelPath, err)
	}
	return &ONNX{tok: tok, session: session}, nil
}

// Close releases the model session.
func (o *ONNX) Close() error {
	if o.session != nil {
		return o.session.Destroy()
	}
	return nil
}

// Model implements Embedder.
func (o *ONNX) Model() string { return ModelName }

// Dimensions implements Embedder.
func (o *ONNX) Dimensions() int { return ModelDims }

// Embed implements Embedder: one batched forward pass over all texts.
func (o *ONNX) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Tokenize and pad to the longest sequence in the batch.
	encoded := make([][]int64, len(texts))
	seqLen := 0
	for i, t := range texts {
		encoded[i] = o.tok.Encode(t, maxSeqLength)
		if len(encoded[i]) > seqLen {
			seqLen = len(encoded[i])
		}
	}
	batch := len(texts)
	inputIDs := make([]int64, batch*seqLen)
	attentionMask := make([]int64, batch*seqLen)
	tokenTypeIDs := make([]int64, batch*seqLen) // all zeros: single segment
	for i, ids := range encoded {
		copy(inputIDs[i*seqLen:], ids)
		for j := range ids {
			attentionMask[i*seqLen+j] = 1
		}
	}

	shape := ort.NewShape(int64(batch), int64(seqLen))
	idsT, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, fmt.Errorf("embed: input tensor: %w", err)
	}
	defer idsT.Destroy()
	maskT, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return nil, fmt.Errorf("embed: mask tensor: %w", err)
	}
	defer maskT.Destroy()
	typeT, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return nil, fmt.Errorf("embed: type tensor: %w", err)
	}
	defer typeT.Destroy()

	outputs := []ort.Value{nil} // allocated by the runtime
	if err := o.session.Run([]ort.Value{idsT, maskT, typeT}, outputs); err != nil {
		return nil, fmt.Errorf("embed: inference: %w", err)
	}
	hidden, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, fmt.Errorf("embed: unexpected output type %T", outputs[0])
	}
	defer hidden.Destroy()

	dims := hidden.GetShape()
	if len(dims) != 3 || int(dims[0]) != batch || int(dims[1]) != seqLen {
		return nil, fmt.Errorf("embed: unexpected output shape %v", dims)
	}
	hiddenSize := int(dims[2])
	data := hidden.GetData()

	// Mean pooling over real (unmasked) tokens, then L2 normalization.
	out := make([][]float32, batch)
	for i := range out {
		vec := make([]float32, hiddenSize)
		count := 0
		for j := 0; j < seqLen; j++ {
			if attentionMask[i*seqLen+j] == 0 {
				continue
			}
			row := data[(i*seqLen+j)*hiddenSize : (i*seqLen+j+1)*hiddenSize]
			for d, v := range row {
				vec[d] += v
			}
			count++
		}
		var norm float64
		for d := range vec {
			vec[d] /= float32(count) // count ≥ 2: [CLS] and [SEP] always present
			norm += float64(vec[d]) * float64(vec[d])
		}
		n := float32(math.Sqrt(norm))
		if n > 0 {
			for d := range vec {
				vec[d] /= n
			}
		}
		out[i] = vec
	}
	return out, nil
}
