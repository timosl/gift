//go:build darwin || freebsd || linux

package ffmpeg

import (
	"fmt"
	"runtime"
	"syscall"

	"github.com/ebitengine/purego"
)

// names are the sonames to try, newest major first. The unversioned names
// are the -dev symlinks, which a device that only plays videos may not have.
func names(base string, majors ...int) []string {
	var out []string
	for _, m := range majors {
		if runtime.GOOS == "darwin" {
			for _, dir := range []string{"", "/opt/homebrew/lib/", "/usr/local/lib/"} {
				out = append(out, fmt.Sprintf("%slib%s.%d.dylib", dir, base, m))
			}
		} else {
			out = append(out, fmt.Sprintf("lib%s.so.%d", base, m))
		}
	}
	if runtime.GOOS == "darwin" {
		return append(out, "/opt/homebrew/lib/lib"+base+".dylib")
	}
	return append(out, "lib"+base+".so")
}

func open(cands []string) (uintptr, error) {
	var err error
	for _, n := range cands {
		var h uintptr
		// RTLD_GLOBAL: libavcodec resolves libavutil's symbols through the
		// handle opened first, and two copies of libavutil in one process
		// would each keep their own log level and CPU flags.
		if h, err = purego.Dlopen(n, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			return h, nil
		}
	}
	return 0, err
}

func bind() (err error) {
	util, err := open(names("avutil", 61, 60, 59, 58, 57))
	if err != nil {
		return fmt.Errorf("%w: libavutil: %v", ErrUnavailable, err)
	}
	codec, err := open(names("avcodec", 63, 62, 61, 60, 59))
	if err != nil {
		return fmt.Errorf("%w: libavcodec: %v", ErrUnavailable, err)
	}
	format, err := open(names("avformat", 63, 62, 61, 60, 59))
	if err != nil {
		return fmt.Errorf("%w: libavformat: %v", ErrUnavailable, err)
	}
	swr, err := open(names("swresample", 7, 6, 5, 4))
	if err != nil {
		return fmt.Errorf("%w: libswresample: %v", ErrUnavailable, err)
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: binding failed: %v", ErrUnavailable, r)
		}
	}()
	purego.RegisterLibFunc(&lib.utilVersion, util, "avutil_version")
	purego.RegisterLibFunc(&lib.codecVersion, codec, "avcodec_version")
	purego.RegisterLibFunc(&lib.formatVersion, format, "avformat_version")
	if lib.abi, err = abiFor(lib.utilVersion()>>16, lib.codecVersion()>>16, lib.formatVersion()>>16); err != nil {
		return err
	}

	purego.RegisterLibFunc(&lib.frameAlloc, util, "av_frame_alloc")
	purego.RegisterLibFunc(&lib.frameFree, util, "av_frame_free")
	purego.RegisterLibFunc(&lib.frameUnref, util, "av_frame_unref")
	purego.RegisterLibFunc(&lib.strerror, util, "av_strerror")
	purego.RegisterLibFunc(&lib.logSetLevel, util, "av_log_set_level")
	purego.RegisterLibFunc(&lib.getPixFmt, util, "av_get_pix_fmt")
	purego.RegisterLibFunc(&lib.getPixFmtName, util, "av_get_pix_fmt_name")
	purego.RegisterLibFunc(&lib.hwdeviceCreate, util, "av_hwdevice_ctx_create")
	purego.RegisterLibFunc(&lib.hwdeviceFind, util, "av_hwdevice_find_type_by_name")
	purego.RegisterLibFunc(&lib.bufferRef, util, "av_buffer_ref")
	purego.RegisterLibFunc(&lib.bufferUnref, util, "av_buffer_unref")
	purego.RegisterLibFunc(&lib.hwframeTransfer, util, "av_hwframe_transfer_data")
	purego.RegisterLibFunc(&lib.optSetInt, util, "av_opt_set_int")
	purego.RegisterLibFunc(&lib.chLayoutDefault, util, "av_channel_layout_default")

	purego.RegisterLibFunc(&lib.openInput, format, "avformat_open_input")
	purego.RegisterLibFunc(&lib.findStreamInfo, format, "avformat_find_stream_info")
	purego.RegisterLibFunc(&lib.findBestStream, format, "av_find_best_stream")
	purego.RegisterLibFunc(&lib.readFrame, format, "av_read_frame")
	purego.RegisterLibFunc(&lib.seekFile, format, "avformat_seek_file")
	purego.RegisterLibFunc(&lib.closeInput, format, "avformat_close_input")

	purego.RegisterLibFunc(&lib.allocContext3, codec, "avcodec_alloc_context3")
	purego.RegisterLibFunc(&lib.freeContext, codec, "avcodec_free_context")
	purego.RegisterLibFunc(&lib.parToContext, codec, "avcodec_parameters_to_context")
	purego.RegisterLibFunc(&lib.open2, codec, "avcodec_open2")
	purego.RegisterLibFunc(&lib.sendPacket, codec, "avcodec_send_packet")
	purego.RegisterLibFunc(&lib.receiveFrame, codec, "avcodec_receive_frame")
	purego.RegisterLibFunc(&lib.flushBuffers, codec, "avcodec_flush_buffers")
	purego.RegisterLibFunc(&lib.packetAlloc, codec, "av_packet_alloc")
	purego.RegisterLibFunc(&lib.packetFree, codec, "av_packet_free")
	purego.RegisterLibFunc(&lib.packetUnref, codec, "av_packet_unref")
	purego.RegisterLibFunc(&lib.packetMoveRef, codec, "av_packet_move_ref")
	purego.RegisterLibFunc(&lib.codecName, codec, "avcodec_get_name")
	purego.RegisterLibFunc(&lib.findDecoder, codec, "avcodec_find_decoder")
	purego.RegisterLibFunc(&lib.dictGet, util, "av_dict_get")

	purego.RegisterLibFunc(&lib.swrAllocSetOpts2, swr, "swr_alloc_set_opts2")
	purego.RegisterLibFunc(&lib.swrInit, swr, "swr_init")
	purego.RegisterLibFunc(&lib.swrConvert, swr, "swr_convert")
	purego.RegisterLibFunc(&lib.swrFree, swr, "swr_free")

	lib.pixYUV420P = lib.getPixFmt("yuv420p")
	lib.pixYUVJ420P = lib.getPixFmt("yuvj420p")
	lib.pixNV12 = lib.getPixFmt("nv12")
	lib.pixYUV420P10 = lib.getPixFmt("yuv420p10le")
	lib.pixDRMPrime = lib.getPixFmt("drm_prime")
	lib.sampleFLT = 3 // AV_SAMPLE_FMT_FLT, unchanged since the enum exists
	lib.hwDRM = lib.hwdeviceFind("drm")
	lib.eagain = -int32(syscall.EAGAIN)
	lib.logSetLevel(16) // AV_LOG_ERROR
	return nil
}
