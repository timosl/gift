package ffmpeg

import "fmt"

// abi holds the byte offsets of the struct fields this package reads or
// writes. FFmpeg's public API is a set of structs, not of accessors, and the
// offsets move between major versions while the function signatures used here
// do not. One table per supported combination of majors is the price of
// binding without cgo; the numbers are printed by testdata/offsets.c compiled
// against the headers of that release, never written by hand.
type abi struct {
	fmtNbStreams, fmtStreams, fmtDuration uintptr

	stIndex, stCodecpar, stTimeBase, stStartTime, stSAR, stAvgFrameRate uintptr
	stMetadata, stDisposition                                           uintptr

	parWidth, parHeight, parSAR, parFieldOrder, parSampleRate, parChLayout uintptr

	ctxHWDeviceCtx, ctxPktTimebase uintptr

	frData, frLinesize, frExtendedData, frWidth, frHeight, frNbSamples, frFormat  uintptr
	frSAR, frPTS, frSampleRate, frFlags, frColorRange, frColorspace, frBestEffort uintptr
	frChLayout, frDuration                                                        uintptr
	// frInterlaced and frTopFirst are the int fields that held the field
	// order before AV_FRAME_FLAG_INTERLACED; zero where frFlags has it.
	frInterlaced, frTopFirst uintptr

	pktStreamIndex uintptr

	chNbChannels  uintptr
	chLayoutBytes uintptr
}

// abis is keyed by the majors of libavutil, libavcodec and libavformat. The
// entries are the output of testdata/offsets.c; within a major FFmpeg only
// appends fields, so one entry covers every minor release of it.
var abis = map[[3]uint32]abi{
	// FFmpeg 5.1: Debian 12 Bookworm and Raspberry Pi OS Bookworm.
	{57, 59, 59}: {
		fmtNbStreams:   44,
		fmtStreams:     48,
		fmtDuration:    72,
		stIndex:        0,
		stCodecpar:     208,
		stTimeBase:     16,
		stStartTime:    24,
		stSAR:          56,
		stAvgFrameRate: 72,
		stMetadata:     64,
		stDisposition:  48,
		parWidth:       56,
		parHeight:      60,
		parSAR:         64,
		parFieldOrder:  72,
		parSampleRate:  116,
		parChLayout:    144,
		ctxHWDeviceCtx: 872,
		ctxPktTimebase: 724,
		frData:         0,
		frLinesize:     64,
		frExtendedData: 96,
		frWidth:        104,
		frHeight:       108,
		frNbSamples:    112,
		frFormat:       116,
		frSAR:          128,
		frPTS:          136,
		frSampleRate:   208,
		frFlags:        316,
		frColorRange:   320,
		frColorspace:   332,
		frBestEffort:   344,
		frChLayout:     448,
		frDuration:     360,
		frInterlaced:   188,
		frTopFirst:     192,
		pktStreamIndex: 36,
		chNbChannels:   4,
		chLayoutBytes:  24,
	},
	// FFmpeg 6.1: Ubuntu 24.04.
	{58, 60, 60}: {
		fmtNbStreams:   44,
		fmtStreams:     48,
		fmtDuration:    72,
		stIndex:        8,
		stCodecpar:     16,
		stTimeBase:     32,
		stStartTime:    40,
		stSAR:          72,
		stAvgFrameRate: 88,
		stMetadata:     80,
		stDisposition:  64,
		parWidth:       56,
		parHeight:      60,
		parSAR:         64,
		parFieldOrder:  72,
		parSampleRate:  116,
		parChLayout:    144,
		ctxHWDeviceCtx: 864,
		ctxPktTimebase: 716,
		frData:         0,
		frLinesize:     64,
		frExtendedData: 96,
		frWidth:        104,
		frHeight:       108,
		frNbSamples:    112,
		frFormat:       116,
		frSAR:          128,
		frPTS:          136,
		frSampleRate:   208,
		frFlags:        316,
		frColorRange:   320,
		frColorspace:   332,
		frBestEffort:   344,
		frChLayout:     448,
		frDuration:     472,
		frInterlaced:   188,
		frTopFirst:     192,
		pktStreamIndex: 36,
		chNbChannels:   4,
		chLayoutBytes:  24,
	},
	// FFmpeg 7.1: Debian 13 Trixie and Raspberry Pi OS Trixie.
	{59, 61, 61}: {
		fmtNbStreams:   44,
		fmtStreams:     48,
		fmtDuration:    104,
		stIndex:        8,
		stCodecpar:     16,
		stTimeBase:     32,
		stStartTime:    40,
		stSAR:          72,
		stAvgFrameRate: 88,
		stMetadata:     80,
		stDisposition:  64,
		parWidth:       72,
		parHeight:      76,
		parSAR:         80,
		parFieldOrder:  96,
		parSampleRate:  152,
		parChLayout:    128,
		ctxHWDeviceCtx: 560,
		ctxPktTimebase: 92,
		frData:         0,
		frLinesize:     64,
		frExtendedData: 96,
		frWidth:        104,
		frHeight:       108,
		frNbSamples:    112,
		frFormat:       116,
		frSAR:          128,
		frPTS:          136,
		frSampleRate:   192,
		frFlags:        292,
		frColorRange:   296,
		frColorspace:   308,
		frBestEffort:   320,
		frChLayout:     408,
		frDuration:     432,
		pktStreamIndex: 36,
		chNbChannels:   4,
		chLayoutBytes:  24,
	},
	// FFmpeg 8.0.
	{60, 62, 62}: {
		fmtNbStreams:   44,
		fmtStreams:     48,
		fmtDuration:    104,
		stIndex:        8,
		stCodecpar:     16,
		stTimeBase:     32,
		stStartTime:    40,
		stSAR:          72,
		stAvgFrameRate: 88,
		stMetadata:     80,
		stDisposition:  64,
		parWidth:       72,
		parHeight:      76,
		parSAR:         80,
		parFieldOrder:  96,
		parSampleRate:  152,
		parChLayout:    128,
		ctxHWDeviceCtx: 560,
		ctxPktTimebase: 92,
		frData:         0,
		frLinesize:     64,
		frExtendedData: 96,
		frWidth:        104,
		frHeight:       108,
		frNbSamples:    112,
		frFormat:       116,
		frSAR:          124,
		frPTS:          136,
		frSampleRate:   180,
		frFlags:        276,
		frColorRange:   280,
		frColorspace:   292,
		frBestEffort:   304,
		frChLayout:     384,
		frDuration:     408,
		pktStreamIndex: 36,
		chNbChannels:   4,
		chLayoutBytes:  24,
	},
	// FFmpeg 9.0: Homebrew on macOS.
	{61, 63, 63}: {
		fmtNbStreams:   44,
		fmtStreams:     48,
		fmtDuration:    104,
		stIndex:        8,
		stCodecpar:     16,
		stTimeBase:     32,
		stStartTime:    40,
		stSAR:          72,
		stAvgFrameRate: 88,
		stMetadata:     80,
		stDisposition:  64,
		parWidth:       72,
		parHeight:      76,
		parSAR:         80,
		parFieldOrder:  96,
		parSampleRate:  152,
		parChLayout:    128,
		ctxHWDeviceCtx: 560,
		ctxPktTimebase: 92,
		frData:         0,
		frLinesize:     64,
		frExtendedData: 96,
		frWidth:        104,
		frHeight:       108,
		frNbSamples:    112,
		frFormat:       116,
		frSAR:          124,
		frPTS:          136,
		frSampleRate:   180,
		frFlags:        276,
		frColorRange:   280,
		frColorspace:   292,
		frBestEffort:   304,
		frChLayout:     384,
		frDuration:     408,
		pktStreamIndex: 36,
		chNbChannels:   4,
		chLayoutBytes:  24,
	},
}

func abiFor(util, codec, format uint32) (abi, error) {
	a, ok := abis[[3]uint32{util, codec, format}]
	if !ok {
		return abi{}, fmt.Errorf("%w: no ABI table for libavutil %d, libavcodec %d, libavformat %d",
			ErrUnavailable, util, codec, format)
	}
	return a, nil
}
