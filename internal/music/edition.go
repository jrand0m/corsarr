package music

import (
	"strings"

	"github.com/jrand0m/corsarr/internal/plugin"
)

// videoFormats are medium formats that hold pictures rather than an audio
// programme; their tracks are never part of an album's files.
var videoFormats = map[string]bool{"dvd": true, "dvd-video": true, "dvd-r video": true, "blu-ray": true, "blu-ray-r": true,
	"hd-dvd": true, "hd dvd": true, "vhs": true, "betamax": true, "vcd": true, "svcd": true, "cvd": true,
	"laserdisc": true, "vhd": true, "ced": true, "umd": true}

// videoMedium reports whether a medium format is a video disc or tape.
func videoMedium(format string) bool {
	return videoFormats[strings.ToLower(strings.TrimSpace(format))]
}

// dataTrack reports whether a track title marks the computer data part of
// an enhanced disc.
func dataTrack(title string) bool {
	return strings.EqualFold(strings.TrimSpace(title), "[data track]")
}

// AudioOnly keeps the audio of an edition: video media and data tracks are
// dropped, then media left without tracks. Media keep their positions.
func AudioOnly(r plugin.ReleaseRecord) plugin.ReleaseRecord {
	media := make([]plugin.MediumRecord, 0, len(r.Media))
	for _, m := range r.Media {
		if videoMedium(m.Format) {
			continue
		}
		if m.Tracks = audioTracks(m.Tracks); len(m.Tracks) > 0 {
			media = append(media, m)
		}
	}
	r.Media = media
	return r
}

func audioTracks(tracks []plugin.TrackRecord) []plugin.TrackRecord {
	out := make([]plugin.TrackRecord, 0, len(tracks))
	for _, t := range tracks {
		if !dataTrack(t.Title) {
			out = append(out, t)
		}
	}
	return out
}

// audioEditions reduces every edition to its audio and keeps those with
// tracks left; when none has any, every reduced edition is kept.
func audioEditions(rels []plugin.ReleaseRecord) []plugin.ReleaseRecord {
	all := make([]plugin.ReleaseRecord, len(rels))
	var withAudio []plugin.ReleaseRecord
	for i, r := range rels {
		all[i] = AudioOnly(r)
		if all[i].TrackCount() > 0 {
			withAudio = append(withAudio, all[i])
		}
	}
	if len(withAudio) == 0 {
		return all
	}
	return withAudio
}
