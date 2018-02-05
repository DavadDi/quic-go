// This file was automatically generated by genny.
// Any changes will be lost if this file is regenerated.
// see https://github.com/cheekybits/genny

package quic

import (
	"fmt"
	"sync"

	"github.com/lucas-clemente/quic-go/internal/protocol"
	"github.com/lucas-clemente/quic-go/internal/wire"
	"github.com/lucas-clemente/quic-go/qerr"
)

type outgoingUniStreamsMap struct {
	mutex sync.RWMutex
	cond  sync.Cond

	streams map[protocol.StreamID]sendStreamI

	nextStream     protocol.StreamID // stream ID of the stream returned by OpenStream(Sync)
	maxStream      protocol.StreamID // the maximum stream ID we're allowed to open
	highestBlocked protocol.StreamID // the highest stream ID that we queued a STREAM_ID_BLOCKED frame for

	newStream         func(protocol.StreamID) sendStreamI
	queueControlFrame func(wire.Frame)

	closeErr error
}

func newOutgoingUniStreamsMap(
	nextStream protocol.StreamID,
	newStream func(protocol.StreamID) sendStreamI,
	queueControlFrame func(wire.Frame),
) *outgoingUniStreamsMap {
	m := &outgoingUniStreamsMap{
		streams:           make(map[protocol.StreamID]sendStreamI),
		nextStream:        nextStream,
		newStream:         newStream,
		queueControlFrame: queueControlFrame,
	}
	m.cond.L = &m.mutex
	return m
}

func (m *outgoingUniStreamsMap) OpenStream() (sendStreamI, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.openStreamImpl()
}

func (m *outgoingUniStreamsMap) OpenStreamSync() (sendStreamI, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for {
		str, err := m.openStreamImpl()
		if err == nil {
			return str, err
		}
		if err != nil && err != qerr.TooManyOpenStreams {
			return nil, err
		}
		m.cond.Wait()
	}
}

func (m *outgoingUniStreamsMap) openStreamImpl() (sendStreamI, error) {
	if m.closeErr != nil {
		return nil, m.closeErr
	}
	if m.nextStream > m.maxStream {
		if m.maxStream == 0 || m.highestBlocked < m.maxStream {
			m.queueControlFrame(&wire.StreamIDBlockedFrame{StreamID: m.maxStream})
			m.highestBlocked = m.maxStream
		}
		return nil, qerr.TooManyOpenStreams
	}
	s := m.newStream(m.nextStream)
	m.streams[m.nextStream] = s
	m.nextStream += 4
	return s, nil
}

func (m *outgoingUniStreamsMap) GetStream(id protocol.StreamID) (sendStreamI, error) {
	if id >= m.nextStream {
		return nil, qerr.Error(qerr.InvalidStreamID, fmt.Sprintf("peer attempted to open stream %d", id))
	}
	m.mutex.RLock()
	s := m.streams[id]
	m.mutex.RUnlock()
	return s, nil
}

func (m *outgoingUniStreamsMap) DeleteStream(id protocol.StreamID) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, ok := m.streams[id]; !ok {
		return fmt.Errorf("Tried to delete unknown stream %d", id)
	}
	delete(m.streams, id)
	return nil
}

func (m *outgoingUniStreamsMap) SetMaxStream(id protocol.StreamID) {
	m.mutex.Lock()
	if id > m.maxStream {
		m.maxStream = id
		m.cond.Broadcast()
	}
	m.mutex.Unlock()
}

func (m *outgoingUniStreamsMap) CloseWithError(err error) {
	m.mutex.Lock()
	m.closeErr = err
	m.cond.Broadcast()
	m.mutex.Unlock()
}
