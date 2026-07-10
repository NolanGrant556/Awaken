package agui

// FrameSeqStride reserves stable sequence slots for the AG-UI frames projected
// from one runtime event or durable semantic record.
const FrameSeqStride uint64 = 16

// FrameSeq returns a unique, monotonic transport sequence for a frame within
// one runtime event or durable semantic record.
func FrameSeq(sourceSeq uint64, frameIndex int) uint64 {
	return sourceSeq*FrameSeqStride + uint64(frameIndex)
}
