package assets

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Q3 configstring indices
const (
	csServerInfo = 0
	csSystemInfo = 1
	csModels     = 32
	csSounds     = 288
	csPlayers    = 544
	csMax        = 1024
)

// Q3 network constants
const (
	maxGentities    = 1024
	gentitynumBits  = 10
	maxClients      = 64
	floatIntBits    = 13
	maxStats        = 16
	maxPersistant   = 16
	maxWeapons      = 16
	maxPowerups     = 16
	numEntityFields = 51
	numPlayerFields = 48
)

// entityFieldBits defines the bit width for each entityState_t netField.
// 0 = float, positive = unsigned int bits, from msg.c entityStateFields[].
var entityFieldBits = [numEntityFields]int{
	32, 0, 0, 0, 0, 0, 0, 0, 0, // pos.trTime, pos.trBase[0..2], pos.trDelta[0..2], apos.trBase[1], apos.trBase[0]
	10, 0, 8, 8, 8, 8,          // event, angles2[1], eType, torsoAnim, eventParm, legsAnim
	10, 8, 19, 10, 8, 8, 0,     // groundEntityNum, pos.trType, eFlags, otherEntityNum, weapon, clientNum, angles[1]
	32, 8, 0, 0, 0, 24, 16,     // pos.trDuration, apos.trType, origin[0..2], solid, powerups
	8, 10, 8, 8,                 // modelindex, otherEntityNum2, loopSound, generic1
	0, 0, 0, 8, 0,              // origin2[2], origin2[0], origin2[1], modelindex2, angles[0]
	32, 32, 32,                  // time, apos.trTime, apos.trDuration
	0, 0, 0, 0,                 // apos.trBase[2], apos.trDelta[0..2]
	32, 0, 0, 0, 32, 16,        // time2, angles[2], angles2[0], angles2[2], constantLight, frame
}

// playerFieldBits defines the bit width for each playerState_t netField.
// 0 = float, negative = signed int, from msg.c playerStateFields[].
var playerFieldBits = [numPlayerFields]int{
	32, 0, 0, 8, 0, 0, 0, 0,    // commandTime, origin[0..1], bobCycle, velocity[0..1], viewangles[1..0]
	-16, 0, 0, 8, -16, 16,      // weaponTime, origin[2], velocity[2], legsTimer, pm_time, eventSequence
	8, 4, 8, 8, 8, 16,          // torsoAnim, movementDir, events[0], legsAnim, events[1], pm_flags
	10, 4, 16, 10, 16, 16, 16,  // groundEntityNum, weaponstate, eFlags, externalEvent, gravity, speed, delta_angles[1]
	8, -8, 8, 8, 8, 8, 8,       // externalEventParm, viewheight, damageEvent, damageYaw, damagePitch, damageCount, generic1
	8, 16, 16, 12, 8, 8,        // pm_type, delta_angles[0], delta_angles[2], torsoTimer, eventParms[0], eventParms[1]
	8, 5, 0, 0, 0, 0, 10, 16,   // clientNum, weapon, viewangles[2], grapplePoint[0..2], jumppad_ent, loopSound
}

// DemoInfo holds extracted asset references from a demo file.
type DemoInfo struct {
	MapName     string
	FSGame      string
	GameType    int
	Models      []string
	Sounds      []string
	PlayerInfos []PlayerInfo
}

// PlayerInfo holds player model information from a demo.
type PlayerInfo struct {
	Model  string
	HModel string
}

// ParseDemo parses a .tvd demo file and extracts asset references.
// TVD header format:
//   - 4 bytes: "TVD1" magic
//   - 4 bytes: protocol version (int32 LE)
//   - 4 bytes: sv_fps (int32 LE)
//   - 4 bytes: maxclients (int32 LE)
//   - null-terminated string: mapname
//   - null-terminated string: timestamp
//   - configstrings: repeated [index:u16][length:u16][data:bytes], terminated by index 0xFFFF
//   - zstd-compressed demo frames follow with additional configstring updates
func ParseDemo(path string) (*DemoInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read demo: %w", err)
	}

	if len(data) < 20 || string(data[0:4]) != "TVD1" {
		return nil, fmt.Errorf("not a TVD file")
	}

	offset := 16 // skip magic(4) + protocol(4) + sv_fps(4) + maxclients(4)

	// Skip mapname (null-terminated)
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	offset++ // skip null

	// Skip timestamp (null-terminated)
	for offset < len(data) && data[offset] != 0 {
		offset++
	}
	offset++ // skip null

	// Read header configstrings
	configstrings := make(map[int]string)
	for offset+4 <= len(data) {
		index := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2

		if index == 0xFFFF {
			break // end of configstrings
		}

		length := int(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2

		if offset+length > len(data) {
			break
		}

		value := string(data[offset : offset+length])
		offset += length

		if value != "" {
			configstrings[index] = value
		}
	}

	// Parse zstd-compressed frame data for configstring updates
	if offset < len(data) {
		parseFrameConfigstrings(data[offset:], configstrings)
	}

	return buildDemoInfo(configstrings), nil
}

// parseFrameConfigstrings decompresses the zstd frame stream and extracts
// configstring updates from each frame. This catches players joining mid-match.
func parseFrameConfigstrings(compressedData []byte, configstrings map[int]string) {
	decoder, err := zstd.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		log.Printf("Demo: zstd decoder init error: %v", err)
		return
	}
	defer decoder.Close()

	decompressed, err := io.ReadAll(decoder)
	if errors.Is(err, zstd.ErrMagicMismatch) {
		err = nil // trailing non-zstd data (file trailer) is expected
	}
	if err != nil {
		log.Printf("Demo: zstd decompress error (read %d bytes): %v", len(decompressed), err)
		if len(decompressed) == 0 {
			return
		}
	}

	pos := 0
	frameCount := 0
	csUpdates := 0

	for pos+4 <= len(decompressed) {
		// Read frame size (4 raw bytes)
		frameSize := int(binary.LittleEndian.Uint32(decompressed[pos:]))
		pos += 4

		if frameSize == 0 || pos+frameSize > len(decompressed) {
			break
		}

		frameData := decompressed[pos : pos+frameSize]
		pos += frameSize
		frameCount++

		// Parse this frame's Huffman-encoded data for configstrings
		n := parseOneFrame(frameData, configstrings)
		csUpdates += n
	}

	if csUpdates > 0 {
		log.Printf("Demo: parsed %d frames, found %d configstring updates", frameCount, csUpdates)
	}
}

// parseOneFrame parses a single Huffman-encoded frame and extracts configstring
// updates. Returns the number of configstrings found.
func parseOneFrame(frameData []byte, configstrings map[int]string) int {
	msg := NewMsgReader(frameData)

	// Server time
	msg.ReadLong()

	// Entity bitmask (MAX_GENTITIES/8 = 128 bytes)
	msg.ReadData(maxGentities / 8)

	// Skip entity deltas: read entity numbers until end marker
	for {
		entityNum := msg.ReadBits(gentitynumBits)
		if entityNum == maxGentities-1 {
			break // end marker
		}
		if msg.Remaining() < 2 {
			return 0 // truncated frame
		}
		skipEntityDelta(msg)
	}

	// Player bitmask (MAX_CLIENTS/8 = 8 bytes)
	playerBitmask := msg.ReadData(maxClients / 8)

	// Skip player deltas
	for i := 0; i < maxClients; i++ {
		if playerBitmask[i>>3]&(1<<uint(i&7)) == 0 {
			continue
		}
		msg.ReadU8() // clientNum
		skipPlayerDelta(msg)
	}

	// Read configstring updates
	csCount := msg.ReadShort()
	if csCount < 0 || csCount > csMax {
		return 0
	}

	for i := 0; i < csCount; i++ {
		csIndex := msg.ReadShort()
		csLen := msg.ReadShort()

		if csLen > 0 && csLen < 8192 {
			csData := msg.ReadData(csLen)
			configstrings[csIndex] = string(csData)
		}
	}

	return csCount
}

// skipEntityDelta skips one MSG_ReadDeltaEntity worth of data.
// Entity fields use zero-value optimization for both floats and ints.
func skipEntityDelta(msg *MsgReader) {
	// Check for remove
	if msg.ReadBits(1) == 1 {
		return
	}
	// Check for no delta
	if msg.ReadBits(1) == 0 {
		return
	}

	lc := int(msg.ReadU8())
	if lc > numEntityFields {
		return
	}

	for i := 0; i < lc; i++ {
		if msg.ReadBits(1) == 0 {
			continue // field unchanged
		}
		bits := entityFieldBits[i]
		if bits == 0 {
			// Float with zero-value check
			if msg.ReadBits(1) == 0 {
				// value is 0.0
			} else if msg.ReadBits(1) == 0 {
				msg.ReadBits(floatIntBits) // integral float
			} else {
				msg.ReadBits(32) // full float
			}
		} else {
			// Integer with zero-value check
			if msg.ReadBits(1) == 0 {
				// value is 0
			} else {
				msg.ReadBits(bits)
			}
		}
	}
}

// skipPlayerDelta skips one MSG_ReadDeltaPlayerstate worth of data.
// Player fields do NOT have the zero-value optimization that entities have.
func skipPlayerDelta(msg *MsgReader) {
	lc := int(msg.ReadU8())
	if lc > numPlayerFields {
		return
	}

	for i := 0; i < lc; i++ {
		if msg.ReadBits(1) == 0 {
			continue // field unchanged
		}
		bits := playerFieldBits[i]
		if bits < 0 {
			bits = -bits
		}
		if bits == 0 {
			// Float — no zero check for players
			if msg.ReadBits(1) == 0 {
				msg.ReadBits(floatIntBits) // integral float
			} else {
				msg.ReadBits(32) // full float
			}
		} else {
			// Integer — no zero check for players
			msg.ReadBits(bits)
		}
	}

	// Arrays section
	if msg.ReadBits(1) == 0 {
		return
	}

	// stats
	if msg.ReadBits(1) != 0 {
		bits := msg.ReadBits(maxStats)
		for i := 0; i < maxStats; i++ {
			if bits&(1<<uint(i)) != 0 {
				msg.ReadShort()
			}
		}
	}

	// persistant
	if msg.ReadBits(1) != 0 {
		bits := msg.ReadBits(maxPersistant)
		for i := 0; i < maxPersistant; i++ {
			if bits&(1<<uint(i)) != 0 {
				msg.ReadShort()
			}
		}
	}

	// ammo
	if msg.ReadBits(1) != 0 {
		bits := msg.ReadBits(maxWeapons)
		for i := 0; i < maxWeapons; i++ {
			if bits&(1<<uint(i)) != 0 {
				msg.ReadShort()
			}
		}
	}

	// powerups
	if msg.ReadBits(1) != 0 {
		bits := msg.ReadBits(maxPowerups)
		for i := 0; i < maxPowerups; i++ {
			if bits&(1<<uint(i)) != 0 {
				msg.ReadLong()
			}
		}
	}
}

func buildDemoInfo(configstrings map[int]string) *DemoInfo {
	info := &DemoInfo{}

	// Parse serverinfo (CS 0)
	if serverInfo, ok := configstrings[csServerInfo]; ok {
		kvs := parseBackslashKV(serverInfo)
		info.MapName = kvs["mapname"]
		info.FSGame = kvs["fs_game"]
		if gt, err := strconv.Atoi(kvs["g_gametype"]); err == nil {
			info.GameType = gt
		}
	}

	// Fallback fs_game from systeminfo (CS 1)
	if info.FSGame == "" {
		if sysInfo, ok := configstrings[csSystemInfo]; ok {
			kvs := parseBackslashKV(sysInfo)
			if fg := kvs["fs_game"]; fg != "" {
				info.FSGame = fg
			}
		}
	}

	// Collect models (CS 32+)
	seen := make(map[string]bool)
	for i := csModels; i < csModels+256; i++ {
		if v, ok := configstrings[i]; ok && v != "" && !strings.HasPrefix(v, "*") {
			if !seen[v] {
				seen[v] = true
				info.Models = append(info.Models, v)
			}
		}
	}

	// Collect sounds (CS 288+)
	seen = make(map[string]bool)
	for i := csSounds; i < csSounds+256; i++ {
		if v, ok := configstrings[i]; ok && v != "" {
			if !seen[v] {
				seen[v] = true
				info.Sounds = append(info.Sounds, v)
			}
		}
	}

	// Collect player infos (CS 544+)
	seen = make(map[string]bool)
	for i := csPlayers; i < csPlayers+64; i++ {
		if v, ok := configstrings[i]; ok && v != "" {
			kvs := parseBackslashKV(v)
			model := kvs["model"]
			hmodel := kvs["hmodel"]
			if model == "" {
				continue
			}
			// Deduplicate by model+hmodel combination
			key := model + "|" + hmodel
			if seen[key] {
				continue
			}
			seen[key] = true
			info.PlayerInfos = append(info.PlayerInfos, PlayerInfo{
				Model:  model,
				HModel: hmodel,
			})
		}
	}

	return info
}

// parseBackslashKV parses a backslash-delimited key-value string.
// Format: \key1\value1\key2\value2
func parseBackslashKV(s string) map[string]string {
	result := make(map[string]string)
	s = strings.TrimPrefix(s, "\\")
	parts := strings.Split(s, "\\")
	for i := 0; i+1 < len(parts); i += 2 {
		result[parts[i]] = parts[i+1]
	}
	return result
}

