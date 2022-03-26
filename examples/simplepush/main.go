package main

import (
	"fmt"

	"github.com/Tencent-RTC/wtn-go"
	"github.com/pion/mediadevices"
	"github.com/pion/mediadevices/pkg/codec/opus"
	"github.com/pion/mediadevices/pkg/codec/x264"
	"github.com/pion/mediadevices/pkg/prop"
	"github.com/pion/webrtc/v3"

	_ "github.com/pion/mediadevices/pkg/driver/audiotest"
	_ "github.com/pion/mediadevices/pkg/driver/videotest"
)

const sdkappid = 1400182497
const secret = ""

func main() {

	x264Params, _ := x264.NewParams()
	x264Params.Preset = x264.PresetUltrafast
	x264Params.BitRate = 1000_000 // 1000kbps
	x264Params.KeyFrameInterval = 60

	opusParams, _ := opus.NewParams()

	codecSelector := mediadevices.NewCodecSelector(
		mediadevices.WithVideoEncoders(&x264Params),
		mediadevices.WithAudioEncoders(&opusParams),
	)

	ms, err := mediadevices.GetUserMedia(mediadevices.MediaStreamConstraints{
		Video: func(c *mediadevices.MediaTrackConstraints) {
			c.Width = prop.Int(640)
			c.Height = prop.Int(480)
		},
		Audio: func(c *mediadevices.MediaTrackConstraints) {
		},
		Codec: codecSelector,
	})

	if err != nil {
		panic(err)
	}

	var video mediadevices.Track
	var audio mediadevices.Track

	for _, track := range ms.GetTracks() {
		track.OnEnded(func(err error) {
			fmt.Printf("Track (ID: %s) ended with error: %v\n",
				track.ID(), err)
		})

		if track.Kind() == webrtc.RTPCodecTypeAudio {
			audio = track
		}
		if track.Kind() == webrtc.RTPCodecTypeVideo {
			video = track
		}
	}

	wtnClient := wtn.NewClient(wtn.Config{
		SDKAppID:   sdkappid,
		Secret:     secret,
		Audio:      true,
		Video:      true,
		AudioTrack: &audio,
		VideoTrack: &video,
	})

	wtnClient.OnConnectionStateChange(func(cs wtn.ConnectionState) {
		fmt.Printf("Connection state changed: %s\n", cs)
		if cs == wtn.ConnectionStateConnected {
			fmt.Println("Connected, start pushing")
		}
	})

	userID := "user01"
	userSig, _ := wtn.GenSig(sdkappid, secret, userID, 3600)

	err = wtnClient.Publish("stream01", userID, userSig)

	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}

	select {}
}
