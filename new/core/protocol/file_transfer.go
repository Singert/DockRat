package protocol

const (
	MsgUploadInit  MessageType = "upload_init"  // 初始化上传请求（含文件名和长度）
	MsgUploadChunk MessageType = "upload_chunk" // 上传数据块
	MsgUploadDone  MessageType = "upload_done"  // 上传完成通知

	MsgDownloadInit  MessageType = "download_init"  // 请求下载文件
	MsgDownloadChunk MessageType = "download_chunk" // 下载数据块
	MsgDownloadDone  MessageType = "download_done"  // 下载完成通知
)

type UploadInitPayload struct {
	Filename string `json:"filename"`
	Filesize int64  `json:"filesize"`
}

type UploadChunkPayload struct {
	Data []byte `json:"data"`
}

type DownloadInitPayload struct {
	Filename string `json:"filename"`
}

type DownloadChunkPayload struct {
	Data []byte `json:"data"`
}
