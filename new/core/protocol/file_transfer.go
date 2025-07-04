package protocol

var (
	fileRecv       func(offset int64, data []byte)
	downloadCloser func()
)

func SetFileReceiver(f func(offset int64, data []byte), closer func()) {
	fileRecv = f
	downloadCloser = closer
}

func OnFileChunk(offset int64, data []byte) {
	if fileRecv != nil {
		fileRecv(offset, data)
	}
}

func OnDownloadDone() {
	if downloadCloser != nil {
		downloadCloser()
	}
}
