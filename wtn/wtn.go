package wtn

import (
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/imroc/req"
	"github.com/pion/webrtc/v3"
	"github.com/tencentyun/tls-sig-api-v2-golang/tencentyun"
)

const WTNBaseURL = "https://signaling.rtc.qcloud.com"

// WTN config struct
type Config struct {
	SDKAppID uint32
	Secret   string

	Audio bool
	Video bool

	// We can use this track to publish
	// If not set, we will create a new track
	AudioTrack webrtc.TrackLocal
	VideoTrack webrtc.TrackLocal
}

type ConnectionState int

const (
	ConnectionStateNew ConnectionState = iota + 1
	ConnectionStateConnected
	ConnectionStateDisconnected
	ConnectionStateFailed
)

type WTNClient struct {
	config     *Config
	videoTrack webrtc.TrackLocal
	audioTrack webrtc.TrackLocal

	mu sync.RWMutex
	pc *webrtc.PeerConnection

	stopURL string

	userID  string
	userSig string

	onConnectionStateChange func(ConnectionState)
}

func GenSig(sdkappid int, secret string, userID string, expire int) (string, error) {
	sig, err := tencentyun.GenUserSig(sdkappid, secret, userID, expire)
	return sig, err
}

func NewClient(config Config) *WTNClient {
	c := &WTNClient{
		config: &config,
	}

	streamID := uuid.NewString()
	if config.Video {
		if config.VideoTrack != nil {
			c.videoTrack = config.VideoTrack
		} else {
			c.videoTrack, _ = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "video/H264"}, uuid.NewString(), streamID)
		}

	}

	if config.Audio {
		c.audioTrack, _ = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: "audio/opus"}, uuid.NewString(), streamID)
	}

	pcconfig := webrtc.Configuration{
		ICEServers:   []webrtc.ICEServer{},
		BundlePolicy: webrtc.BundlePolicyMaxBundle,
		SDPSemantics: webrtc.SDPSemanticsUnifiedPlan,
	}
	c.pc, _ = webrtc.NewPeerConnection(pcconfig)

	return c
}

func (c *WTNClient) OnConnectionStateChange(f func(ConnectionState)) {
	c.mu.Lock()
	c.onConnectionStateChange = f
	c.mu.Unlock()
}

func (c *WTNClient) Publish(streamID string, userID string, userSig string) error {

	c.userID = userID
	c.userSig = userSig

	pushURL := fmt.Sprintf("%s/v1/push/%s", WTNBaseURL, streamID)
	params := req.QueryParam{
		"sdkappid": c.config.SDKAppID,
		"userid":   userID,
		"usersig":  userSig,
	}

	fmt.Println("pushURL: ", pushURL)
	fmt.Println("params: ", params)

	if c.config.Audio {
		c.pc.AddTransceiverFromTrack(c.audioTrack, webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
	}

	if c.config.Video {
		c.pc.AddTransceiverFromTrack(c.videoTrack, webrtc.RtpTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
	}

	c.pc.OnConnectionStateChange(func(pcs webrtc.PeerConnectionState) {
		switch pcs {
		case webrtc.PeerConnectionStateConnected:
			c.onConnectionStateChange(ConnectionStateConnected)
		case webrtc.PeerConnectionStateDisconnected:
			c.onConnectionStateChange(ConnectionStateDisconnected)
		case webrtc.PeerConnectionStateFailed:
			c.onConnectionStateChange(ConnectionStateFailed)
		}
	})

	sdp, err := c.pc.CreateOffer(nil)
	if err != nil {
		return err
	}
	err = c.pc.SetLocalDescription(sdp)
	if err != nil {
		return err
	}

	res, err := req.Post(pushURL, req.QueryParam{
		"sdkappid": c.config.SDKAppID,
		"userid":   userID,
		"usersig":  userSig,
	}, req.Header{
		"Content-Type": "application/sdp",
	}, sdp.SDP)

	if err != nil {
		return err
	}

	if res.Response().StatusCode > 201 {
		return errors.New(res.Response().Status)
	}

	c.stopURL = res.Response().Header.Get("Location")

	fmt.Println("stopURL: ", c.stopURL)

	answer := webrtc.SessionDescription{
		SDP:  res.String(),
		Type: webrtc.SDPTypeAnswer,
	}
	err = c.pc.SetRemoteDescription(answer)

	return err
}

func (c *WTNClient) Stop() error {

	res, err := req.Delete(c.stopURL)
	if err != nil {
		return err
	}
	if res.Response().StatusCode > 201 {
		return errors.New(res.Response().Status)
	}

	err = c.pc.Close()
	return err
}
