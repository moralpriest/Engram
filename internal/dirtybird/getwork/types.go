package getwork

// Job is the subset of derohe rpc.GetBlockTemplate_Result the miner uses.
// The daemon/pool pushes one of these over the websocket roughly every 500ms
// and immediately when the template changes.
type Job struct {
	JobID             string `json:"jobid"`
	Blockhashing_blob string `json:"blockhashing_blob"`
	Difficulty        string `json:"difficulty"`
	Difficultyuint64  uint64 `json:"difficultyuint64"`
	Height            uint64 `json:"height"`
	Blocks            uint64 `json:"blocks"`
	MiniBlocks        uint64 `json:"miniblocks"`
	Rejected          uint64 `json:"rejected"`
	LastError         string `json:"lasterror"`
}

// Submit is derohe rpc.SubmitBlock_Params: the share submission frame.
// The server never acks a submit; outcomes surface in later Job counters.
type Submit struct {
	JobID string `json:"jobid"`
	Blob  string `json:"mbl_blob"`
	Epoch uint64 `json:"-"`
}
