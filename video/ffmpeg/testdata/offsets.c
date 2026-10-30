// offsets prints the ABI table entry of video/ffmpeg for the FFmpeg headers it
// is compiled against:
//
//	cc -I<prefix>/include offsets.c -o offsets && ./offsets
//
// Only the structs are read; nothing is linked.
#include <stdio.h>
#include <stddef.h>
#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libavutil/frame.h>
#include <libavutil/version.h>

#define O(name, T, f) printf("\t\t" #name ": %zu,\n", offsetof(T, f))

int main(void) {
	printf("\t// FFmpeg with libavutil %d, libavcodec %d, libavformat %d.\n",
		LIBAVUTIL_VERSION_MAJOR, LIBAVCODEC_VERSION_MAJOR, LIBAVFORMAT_VERSION_MAJOR);
	printf("\t{%d, %d, %d}: {\n", LIBAVUTIL_VERSION_MAJOR, LIBAVCODEC_VERSION_MAJOR, LIBAVFORMAT_VERSION_MAJOR);
	O(fmtNbStreams, AVFormatContext, nb_streams);
	O(fmtStreams, AVFormatContext, streams);
	O(fmtDuration, AVFormatContext, duration);
	O(stIndex, AVStream, index);
	O(stCodecpar, AVStream, codecpar);
	O(stTimeBase, AVStream, time_base);
	O(stStartTime, AVStream, start_time);
	O(stSAR, AVStream, sample_aspect_ratio);
	O(stAvgFrameRate, AVStream, avg_frame_rate);
	O(stMetadata, AVStream, metadata);
	O(stDisposition, AVStream, disposition);
	O(parWidth, AVCodecParameters, width);
	O(parHeight, AVCodecParameters, height);
	O(parSAR, AVCodecParameters, sample_aspect_ratio);
	O(parFieldOrder, AVCodecParameters, field_order);
	O(parSampleRate, AVCodecParameters, sample_rate);
	O(parChLayout, AVCodecParameters, ch_layout);
	O(ctxHWDeviceCtx, AVCodecContext, hw_device_ctx);
	O(ctxPktTimebase, AVCodecContext, pkt_timebase);
	O(frData, AVFrame, data);
	O(frLinesize, AVFrame, linesize);
	O(frExtendedData, AVFrame, extended_data);
	O(frWidth, AVFrame, width);
	O(frHeight, AVFrame, height);
	O(frNbSamples, AVFrame, nb_samples);
	O(frFormat, AVFrame, format);
	O(frSAR, AVFrame, sample_aspect_ratio);
	O(frPTS, AVFrame, pts);
	O(frSampleRate, AVFrame, sample_rate);
	O(frFlags, AVFrame, flags);
	O(frColorRange, AVFrame, color_range);
	O(frColorspace, AVFrame, colorspace);
	O(frBestEffort, AVFrame, best_effort_timestamp);
	O(frChLayout, AVFrame, ch_layout);
#if LIBAVUTIL_VERSION_MAJOR >= 58
	O(frDuration, AVFrame, duration);
#else
	O(frDuration, AVFrame, pkt_duration);
#endif
#if LIBAVUTIL_VERSION_MAJOR < 59
	// Before AV_FRAME_FLAG_INTERLACED (libavutil 58.29) the field order
	// lived in two ints of their own.
	O(frInterlaced, AVFrame, interlaced_frame);
	O(frTopFirst, AVFrame, top_field_first);
#endif
	O(pktStreamIndex, AVPacket, stream_index);
	O(chNbChannels, AVChannelLayout, nb_channels);
	printf("\t\tchLayoutBytes: %zu,\n", sizeof(AVChannelLayout));
	printf("\t},\n");
	return 0;
}
