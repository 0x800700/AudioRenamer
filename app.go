package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"github.com/dhowden/tag"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// --- Struct Definitions ---

type LocalTrack struct {
	Path         string `json:"path"`
	OriginalName string `json:"originalName"`
	TagArtist    string `json:"tagArtist"`
	TagTitle     string `json:"tagTitle"`
}

type BandcampAlbum struct {
	Artist    string          `json:"artist"`
	TrackInfo []BandcampTrack `json:"trackinfo"`
	Current   CurrentInfo     `json:"current"`
}

type CurrentInfo struct {
	Title string `json:"title"`
}

type BandcampTrack struct {
	Title    string  `json:"title"`
	Artist   *string `json:"artist"`
	TrackNum int     `json:"track_num"`
}

type MatchedTrack struct {
	LocalPath       string  `json:"localPath"`
	OriginalName    string  `json:"originalName"`
	ProposedNewName string  `json:"proposedNewName"`
	Confidence      float64 `json:"confidence"`
	Status          string  `json:"status"`
}

type AlbumData struct {
	Artist string
	Title  string
	Tracks []AlbumTrack
	Source string
}

type AlbumTrack struct {
	Title            string
	Artist           string
	TrackNum         int
	TrackNumExplicit bool
	TrackID          int
}

type templateCandidate struct {
	Artist     string
	Title      string
	Track      string
	BPM        string
	BPMStyle   string
	Confidence float64
}

var (
	reLegacyTemplate = regexp.MustCompile(`^(?i)(?P<artist>.+?)\s+-\s+(?P<album>.+?)\s+-\s+(?P<track>\d+)\s+(?P<title>.+?)(?:\s+\((?P<bpm>\d+)\))?$`)
	reBPM            = regexp.MustCompile(`(?i)(\d{2,3})\s*[-_ ]*bpm`)
	reTrackPrefix    = regexp.MustCompile(`^\s*(\d{1,2})[.\s_-]+(.+)$`)
	reTrackOnly      = regexp.MustCompile(`^\s*(\d{1,2})\b`)
	reDigitsOnly     = regexp.MustCompile(`^\d{1,3}$`)
	reLabelKeywords  = regexp.MustCompile(`(?i)\b(records?|recordings|music|label|netlabel|rec|recs)\b`)
	reMultiDash      = regexp.MustCompile(`--+`)
	reSpacedDash     = regexp.MustCompile(`\s+-\s+`)
	reSpaces         = regexp.MustCompile(`\s+`)
	reNonWordSpace   = regexp.MustCompile(`[^a-z0-9]+`)
)

func normalizeTemplateBase(raw string) string {
	s := raw
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "—", " - ")
	s = strings.ReplaceAll(s, "–", " - ")
	s = reMultiDash.ReplaceAllString(s, " - ")
	s = reSpacedDash.ReplaceAllString(s, " - ")
	s = reSpaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func extractBPM(raw string) (string, string, string) {
	loc := reBPM.FindStringSubmatchIndex(raw)
	if loc == nil {
		return "", "", raw
	}
	num := raw[loc[2]:loc[3]]
	match := raw[loc[0]:loc[1]]
	style := "space"
	if strings.Contains(match, "Bpm") && !strings.Contains(match, " bpm") && !strings.Contains(match, " Bpm") {
		style = "compact"
	}
	cleaned := strings.TrimSpace(raw[:loc[0]] + " " + raw[loc[1]:])
	return num, style, cleaned
}

func formatBPM(bpm string, style string) string {
	if bpm == "" {
		return ""
	}
	if style == "compact" {
		return fmt.Sprintf("(%sBpm)", bpm)
	}
	return fmt.Sprintf("(%s bpm)", bpm)
}

func extractTrackPrefix(s string) (string, string, bool) {
	m := reTrackPrefix.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

func extractTrackNumber(s string) string {
	if m := reTrackOnly.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func isCatalogCodeToken(token string) bool {
	if len(token) < 4 {
		return false
	}
	hasDigit := false
	hasLetter := false
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		default:
			return false
		}
	}
	return hasDigit && hasLetter
}

func stripTrailingCodeTokens(s string) string {
	tokens := strings.Fields(s)
	for len(tokens) > 0 {
		last := tokens[len(tokens)-1]
		if len(last) == 1 || isCatalogCodeToken(last) {
			tokens = tokens[:len(tokens)-1]
			continue
		}
		break
	}
	return strings.Join(tokens, " ")
}

func isLabelPart(s string) bool {
	return reLabelKeywords.MatchString(strings.ToLower(s))
}

func splitTemplateParts(s string) []string {
	rawParts := strings.Split(s, " - ")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 1 && isCatalogCodeToken(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func splitTemplatePartsLoose(s string) []string {
	if strings.Contains(s, " - ") {
		return splitTemplateParts(s)
	}
	if strings.Count(s, "-") < 2 {
		return []string{strings.TrimSpace(s)}
	}
	rawParts := strings.Split(s, "-")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 1 && isCatalogCodeToken(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func parseTemplateFromParts(parts []string) (templateCandidate, bool) {
	if len(parts) == 0 {
		return templateCandidate{}, false
	}
	if len(parts) >= 3 && reDigitsOnly.MatchString(parts[0]) {
		return templateCandidate{
			Artist:     parts[1],
			Title:      strings.Join(parts[2:], " - "),
			Track:      parts[0],
			Confidence: 0.8,
		}, true
	}
	if t, rest, ok := extractTrackPrefix(parts[len(parts)-1]); ok {
		return templateCandidate{
			Artist:     parts[0],
			Title:      rest,
			Track:      t,
			Confidence: 0.8,
		}, true
	}
	if t, rest, ok := extractTrackPrefix(parts[0]); ok {
		if rest != "" {
			title := ""
			if len(parts) == 2 {
				title = parts[1]
			} else {
				title = parts[len(parts)-1]
			}
			return templateCandidate{
				Artist:     rest,
				Title:      title,
				Track:      t,
				Confidence: 0.7,
			}, true
		}
	}
	if len(parts) >= 3 {
		if isLabelPart(parts[0]) {
			return templateCandidate{
				Artist:     parts[1],
				Title:      strings.Join(parts[2:], " - "),
				Confidence: 0.95,
			}, true
		}
		return templateCandidate{
			Artist:     parts[0],
			Title:      strings.Join(parts[1:], " - "),
			Confidence: 0.55,
		}, true
	}
	if len(parts) == 2 {
		return templateCandidate{
			Artist:     parts[0],
			Title:      parts[1],
			Confidence: 0.6,
		}, true
	}
	return templateCandidate{}, false
}

func normalizeVsTokens(tokens []string) []string {
	for i, t := range tokens {
		if strings.EqualFold(t, "vs") || strings.EqualFold(t, "vs.") {
			tokens[i] = "Vs."
		}
	}
	return tokens
}

func parseTemplateFromTokens(s string) (templateCandidate, bool) {
	track, rest, hasTrack := extractTrackPrefix(s)
	base := s
	if hasTrack {
		base = rest
	}
	base = strings.TrimSpace(base)
	base = stripTrailingCodeTokens(base)
	if base == "" {
		return templateCandidate{}, false
	}
	tokens := strings.Fields(base)
	if len(tokens) >= 2 && strings.EqualFold(tokens[0], tokens[1]) {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return templateCandidate{}, false
	}
	for i, t := range tokens {
		if strings.EqualFold(t, "vs") || strings.EqualFold(t, "vs.") {
			if i+1 < len(tokens)-1 {
				artistTokens := normalizeVsTokens(tokens[:i+2])
				titleTokens := tokens[i+2:]
				return templateCandidate{
					Artist:     strings.Join(artistTokens, " "),
					Title:      strings.Join(titleTokens, " "),
					Track:      track,
					Confidence: 0.55,
				}, true
			}
		}
	}
	for i := len(tokens) - 1; i >= 0; i-- {
		if tokens[i] == "&" && i+1 < len(tokens)-1 {
			artistTokens := tokens[:i+2]
			titleTokens := tokens[i+2:]
			return templateCandidate{
				Artist:     strings.Join(artistTokens, " "),
				Title:      strings.Join(titleTokens, " "),
				Track:      track,
				Confidence: 0.5,
			}, true
		}
	}
	if len(tokens) >= 2 {
		return templateCandidate{
			Artist:     tokens[0],
			Title:      strings.Join(tokens[1:], " "),
			Track:      track,
			Confidence: 0.4,
		}, true
	}
	return templateCandidate{}, false
}

func chooseBestCandidate(candidates ...templateCandidate) templateCandidate {
	best := templateCandidate{Confidence: -1}
	for _, c := range candidates {
		if c.Artist == "" || c.Title == "" {
			continue
		}
		if c.Confidence > best.Confidence {
			best = c
		}
	}
	return best
}

func appendBPMIfMissing(title string, bpm string, style string) string {
	if bpm == "" {
		return title
	}
	lower := strings.ToLower(title)
	if strings.Contains(lower, "bpm") {
		return title
	}
	return strings.TrimSpace(title) + " " + formatBPM(bpm, style)
}

func formatTrackPrefix(track string) string {
	if track == "" {
		return ""
	}
	if n, err := strconv.Atoi(track); err == nil {
		return fmt.Sprintf("%02d. ", n)
	}
	return track + ". "
}

func cleanTrackTitle(title string, trackNum int) string {
	cleaned := title
	prefixes := []string{
		fmt.Sprintf("%d. ", trackNum),
		fmt.Sprintf("%02d. ", trackNum),
		fmt.Sprintf("%d - ", trackNum),
		fmt.Sprintf("%02d - ", trackNum),
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = strings.TrimPrefix(cleaned, prefix)
			break
		}
	}
	return strings.TrimSpace(cleaned)
}

func normalizeForMatch(raw string) string {
	if raw == "" {
		return ""
	}
	_, _, base := extractBPM(raw)
	base = normalizeTemplateBase(base)
	if _, rest, ok := extractTrackPrefix(base); ok {
		base = rest
	}
	base = stripTrailingCodeTokens(base)
	base = strings.ToLower(base)
	base = reNonWordSpace.ReplaceAllString(base, " ")
	base = reSpaces.ReplaceAllString(base, " ")
	return strings.TrimSpace(base)
}

func hasTokenOverlap(a string, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(aTokens))
	for _, t := range aTokens {
		set[t] = struct{}{}
	}
	for _, t := range bTokens {
		if _, ok := set[t]; ok {
			return true
		}
	}
	return false
}

func parseLegacyTemplate(base string) (templateCandidate, bool) {
	matches := reLegacyTemplate.FindStringSubmatch(base)
	if matches == nil {
		return templateCandidate{}, false
	}
	result := make(map[string]string)
	for i, name := range reLegacyTemplate.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}
	cand := templateCandidate{
		Artist:     result["artist"],
		Title:      result["title"],
		Track:      result["track"],
		Confidence: 0.9,
	}
	if bpm := strings.TrimSpace(result["bpm"]); bpm != "" {
		cand.BPM = bpm
		cand.BPMStyle = "compact"
	}
	return cand, true
}

func buildTemplateCandidate(localTrack LocalTrack) templateCandidate {
	ext := filepath.Ext(localTrack.OriginalName)
	base := strings.TrimSuffix(localTrack.OriginalName, ext)
	bpm, bpmStyle, baseNoBpm := extractBPM(base)
	normalized := normalizeTemplateBase(baseNoBpm)
	normalized = stripTrailingCodeTokens(normalized)
	parts := splitTemplateParts(normalized)
	if len(parts) <= 1 {
		parts = splitTemplatePartsLoose(normalized)
	}
	if bpm == "" && len(parts) >= 3 && reDigitsOnly.MatchString(parts[len(parts)-1]) {
		// Ambiguous numeric tail without BPM marker: keep original.
		return templateCandidate{}
	}

	var legacy templateCandidate
	if cand, ok := parseLegacyTemplate(baseNoBpm); ok {
		legacy = cand
	}
	var partsCand templateCandidate
	if cand, ok := parseTemplateFromParts(parts); ok {
		partsCand = cand
	}
	var tokenCand templateCandidate
	if cand, ok := parseTemplateFromTokens(normalized); ok {
		tokenCand = cand
	}
	fileCand := chooseBestCandidate(legacy, partsCand, tokenCand)
	fileCand.BPM = bpm
	fileCand.BPMStyle = bpmStyle

	tagArtist := strings.TrimSpace(localTrack.TagArtist)
	tagTitle := strings.TrimSpace(localTrack.TagTitle)
	if tagArtist != "" || tagTitle != "" {
		if fileCand.Artist == "" || fileCand.Title == "" {
			tagCand := templateCandidate{
				Artist:     tagArtist,
				Title:      tagTitle,
				BPM:        bpm,
				BPMStyle:   bpmStyle,
				Confidence: 0.85,
			}
			filenameNorm := normalizeForMatch(baseNoBpm)
			if tagCand.Artist != "" && tagCand.Title != "" {
				tagNorm := normalizeForMatch(tagCand.Artist + " - " + tagCand.Title)
				if hasTokenOverlap(tagNorm, filenameNorm) {
					return tagCand
				}
			}
		}
	}

	return fileCand
}

// --- Core Functions ---

func (a *App) SelectFolder() ([]LocalTrack, error) {
	dirPath, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Album Folder",
	})
	if err != nil {
		return nil, err
	}
	if dirPath == "" {
		return []LocalTrack{}, nil
	}

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var localTracks []LocalTrack
	validExts := map[string]bool{".flac": true, ".mp3": true, ".wav": true, ".aiff": true}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(file.Name()))
		if validExts[ext] {
			track := LocalTrack{
				Path:         filepath.Join(dirPath, file.Name()),
				OriginalName: file.Name(),
			}

			// Try to read tags
			f, err := os.Open(track.Path)
			if err == nil {
				m, err := tag.ReadFrom(f)
				if err == nil {
					track.TagArtist = m.Artist()
					track.TagTitle = m.Title()
				}
				f.Close()
			}

			localTracks = append(localTracks, track)
		}
	}
	return localTracks, nil
}

func (a *App) GenerateTemplateRenames(localTracks []LocalTrack, format string) ([]MatchedTrack, error) {
	var matchedTracks []MatchedTrack

	for _, localTrack := range localTracks {
		cand := buildTemplateCandidate(localTrack)
		track := MatchedTrack{
			LocalPath:       localTrack.Path,
			OriginalName:    localTrack.OriginalName,
			ProposedNewName: localTrack.OriginalName,
			Confidence:      0,
			Status:          "No Match",
		}

		if cand.Artist != "" && cand.Title != "" {
			title := appendBPMIfMissing(cand.Title, cand.BPM, cand.BPMStyle)
			var base string
			if format == "Track. Title" {
				base = title
			} else {
				if cand.Artist != "" {
					base = fmt.Sprintf("%s - %s", cand.Artist, title)
				} else {
					base = title
				}
			}
			prefix := formatTrackPrefix(cand.Track)
			proposed := strings.TrimSpace(prefix + base)
			proposed = reSpaces.ReplaceAllString(proposed, " ")
			ext := filepath.Ext(localTrack.OriginalName)
			if ext != "" {
				proposed += ext
			}
			track.ProposedNewName = proposed
			track.Confidence = cand.Confidence
			track.Status = "Matched"
		}
		matchedTracks = append(matchedTracks, track)
	}
	return matchedTracks, nil
}

func (a *App) FetchAndMatchTracks(url string, localTracks []LocalTrack) ([]MatchedTrack, error) {
	album, err := a.fetchAlbumData(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch or parse album data: %w", err)
	}

	var matchedTracks []MatchedTrack
	sm := metrics.NewSorensenDice()
	sm.CaseSensitive = false

	availableLocalTracks := make([]LocalTrack, len(localTracks))
	copy(availableLocalTracks, localTracks)

	log.Printf("Album URL: %s", url)
	log.Printf("Album Artist: %s", album.Artist)
	lowerAlbumArtist := strings.ToLower(album.Artist)
	isVA := strings.Contains(lowerAlbumArtist, "various") || strings.Contains(lowerAlbumArtist, "v.a.") || strings.Contains(lowerAlbumArtist, "va ") || strings.Contains(lowerAlbumArtist, "various artists") || strings.Contains(url, "/va-")
	log.Printf("Is VA Album (calculated): %t", isVA)

	for _, albumTrack := range album.Tracks {
		log.Printf("Processing Album Track: %s (Num: %d)", albumTrack.Title, albumTrack.TrackNum)

		if len(availableLocalTracks) == 0 {
			break
		}

		var bestMatchIndex = -1
		var bestMatchRating float64 = -1.0

		candidateIndexes := make([]int, len(availableLocalTracks))
		for i := range availableLocalTracks {
			candidateIndexes[i] = i
		}

		for _, i := range candidateIndexes {
			local := availableLocalTracks[i]
			// **FIXED**: Use the full original name (without extension) for comparison.
			localNameToCompare := strings.TrimSuffix(local.OriginalName, filepath.Ext(local.OriginalName))
			normalizedLocal := normalizeForMatch(localNameToCompare)

			// Calculate similarity based on filename
			ratingFilename := 0.0
			normalizedTitle := normalizeForMatch(albumTrack.Title)
			if normalizedTitle != "" && hasTokenOverlap(normalizedTitle, normalizedLocal) {
				ratingFilename = strutil.Similarity(normalizedTitle, normalizedLocal, sm)
			}
			normalizedFull := ""
			if strings.TrimSpace(albumTrack.Artist) != "" {
				normalizedFull = normalizeForMatch(albumTrack.Artist + " - " + albumTrack.Title)
			} else if album.Artist != "" {
				normalizedFull = normalizeForMatch(album.Artist + " - " + albumTrack.Title)
			}
			if normalizedFull != "" && hasTokenOverlap(normalizedFull, normalizedLocal) {
				if score := strutil.Similarity(normalizedFull, normalizedLocal, sm); score > ratingFilename {
					ratingFilename = score
				}
			}

			// Calculate similarity based on tags (if available)
			ratingTags := 0.0
			if local.TagTitle != "" {
				tagBase := normalizeForMatch(local.TagTitle)
				if tagBase != "" && normalizedTitle != "" && hasTokenOverlap(normalizedTitle, tagBase) {
					ratingTags = strutil.Similarity(normalizedTitle, tagBase, sm)
				}
				if local.TagArtist != "" && normalizedFull != "" {
					tagFull := normalizeForMatch(local.TagArtist + " - " + local.TagTitle)
					if tagFull != "" && hasTokenOverlap(normalizedFull, tagFull) {
						if score := strutil.Similarity(normalizedFull, tagFull, sm); score > ratingTags {
							ratingTags = score
						}
					}
				}
			}

			// Use the higher of the two ratings
			rating := ratingFilename
			if ratingTags > rating {
				rating = ratingTags
				log.Printf("  Higher match found using tags for '%s': %f", local.OriginalName, rating)
			}

			log.Printf("  Comparing Album '%s' with Local '%s' (Tags: '%s' - '%s')", albumTrack.Title, localNameToCompare, local.TagArtist, local.TagTitle)
			log.Printf("  Similarity Rating: %f", rating)

			if albumTrack.TrackNumExplicit && albumTrack.TrackNum > 0 {
				localNum := extractTrackNumber(localNameToCompare)
				if localNum != "" {
					if n, err := strconv.Atoi(localNum); err == nil && n == albumTrack.TrackNum {
						if rating < 0.9 {
							rating += 0.12
							if rating > 1.0 {
								rating = 1.0
							}
						}
					}
				}
			}

			if rating > bestMatchRating {
				bestMatchRating = rating
				bestMatchIndex = i
			}
		}

		minConfidence := 0.0
		if strings.EqualFold(album.Source, "Beatport") {
			minConfidence = 0.25
		}
		if bestMatchIndex != -1 && bestMatchRating >= minConfidence {
			matchedLocalTrack := availableLocalTracks[bestMatchIndex]
			ext := filepath.Ext(matchedLocalTrack.OriginalName)

			localTrackNum := extractTrackNumber(strings.TrimSuffix(matchedLocalTrack.OriginalName, ext))
			trackNumForName := albumTrack.TrackNum
			if localTrackNum != "" {
				if n, err := strconv.Atoi(localTrackNum); err == nil {
					if strings.EqualFold(album.Source, "Beatport") && albumTrack.TrackNumExplicit && albumTrack.TrackNum != n {
						trackNumForName = n
					} else if !albumTrack.TrackNumExplicit {
						trackNumForName = n
					}
				}
			}

			trackNumForClean := 0
			if albumTrack.TrackNumExplicit {
				trackNumForClean = albumTrack.TrackNum
			}

			// 1. Clean the title to handle cases where the track number is duplicated by Bandcamp
			cleanedTitle := albumTrack.Title
			if trackNumForClean > 0 {
				cleanedTitle = cleanTrackTitle(albumTrack.Title, trackNumForClean)
			}

			// 2. Now apply the logic to construct the name
			var proposedName string
			albumArtistFromTitle := strings.TrimSpace(album.Artist)
			if albumArtistFromTitle == "" {
				albumTitleParts := strings.SplitN(album.Title, " - ", 2)
				if len(albumTitleParts) > 1 {
					albumArtistFromTitle = albumTitleParts[0]
				}
			}

			// Use the cleaned title for the check
			if strings.Contains(cleanedTitle, "-") {
				// Title is likely "Artist - Title", so use it as is.
				proposedName = fmt.Sprintf("%02d. %s%s", trackNumForName, cleanedTitle, ext)
			} else if strings.TrimSpace(albumTrack.Artist) != "" {
				// Track contains artist info, so use it.
				proposedName = fmt.Sprintf("%02d. %s - %s%s", trackNumForName, strings.TrimSpace(albumTrack.Artist), cleanedTitle, ext)
			} else if albumArtistFromTitle != "" {
				// Title does not contain artist, so prepend the artist from the album title.
				proposedName = fmt.Sprintf("%02d. %s - %s%s", trackNumForName, albumArtistFromTitle, cleanedTitle, ext)
			} else {
				// Fallback: can't find artist anywhere, just use the cleaned title.
				proposedName = fmt.Sprintf("%02d. %s%s", trackNumForName, cleanedTitle, ext)
			}

			matchedTracks = append(matchedTracks, MatchedTrack{
				LocalPath:       matchedLocalTrack.Path,
				OriginalName:    matchedLocalTrack.OriginalName,
				ProposedNewName: proposedName,
				Confidence:      bestMatchRating,
				Status:          fmt.Sprintf("%s Match", album.Source),
			})

			availableLocalTracks = append(availableLocalTracks[:bestMatchIndex], availableLocalTracks[bestMatchIndex+1:]...)
		}
	}

	return matchedTracks, nil
}

func (a *App) RenameMatchedTracks(tracks []MatchedTrack) (string, error) {
	renamedCount := 0
	for _, track := range tracks {
		if track.OriginalName == track.ProposedNewName {
			continue
		}

		newPath := filepath.Join(filepath.Dir(track.LocalPath), track.ProposedNewName)

		if _, err := os.Stat(newPath); !os.IsNotExist(err) {
			log.Printf("Skipping rename for %s: target file %s already exists", track.OriginalName, track.ProposedNewName)
			continue
		}

		if err := os.Rename(track.LocalPath, newPath); err != nil {
			log.Printf("Error renaming %s to %s: %v", track.LocalPath, newPath, err)
			continue
		}
		renamedCount++
	}
	return fmt.Sprintf("Successfully renamed %d track(s).", renamedCount), nil
}

func (a *App) fetchAlbumData(url string) (*AlbumData, error) {
	lower := strings.ToLower(url)
	if strings.Contains(lower, "beatport.com") {
		return a.fetchBeatportData(url)
	}
	return a.fetchBandcampAlbum(url)
}

func (a *App) fetchBandcampAlbum(url string) (*AlbumData, error) {
	album, err := a.fetchBandcampData(url)
	if err != nil {
		return nil, err
	}
	tracks := make([]AlbumTrack, 0, len(album.TrackInfo))
	for _, bcTrack := range album.TrackInfo {
		artist := ""
		if bcTrack.Artist != nil {
			artist = *bcTrack.Artist
		}
		tracks = append(tracks, AlbumTrack{
			Title:            bcTrack.Title,
			Artist:           artist,
			TrackNum:         bcTrack.TrackNum,
			TrackNumExplicit: true,
			TrackID:          0,
		})
	}
	return &AlbumData{
		Artist: album.Artist,
		Title:  album.Current.Title,
		Tracks: tracks,
		Source: "Bandcamp",
	}, nil
}

func (a *App) fetchBandcampData(url string) (*BandcampAlbum, error) {
	res, err := getWithUserAgent(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	data := doc.Find("script[data-tralbum]").AttrOr("data-tralbum", "")
	if data == "" {
		return nil, fmt.Errorf("could not find album data on page")
	}

	var album BandcampAlbum
	if err := json.Unmarshal([]byte(data), &album); err != nil {
		return nil, fmt.Errorf("failed to unmarshal album data: %w", err)
	}

	return &album, nil
}

func (a *App) fetchBeatportData(url string) (*AlbumData, error) {
	res, err := getWithUserAgent(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return nil, err
	}

	album := AlbumData{Source: "Beatport"}
	releaseID := extractBeatportReleaseID(url)

	if ogTitle := strings.TrimSpace(doc.Find("meta[property='og:title']").AttrOr("content", "")); ogTitle != "" {
		title, artist := parseBeatportMetaTitle(ogTitle)
		if album.Title == "" {
			album.Title = title
		}
		if album.Artist == "" {
			album.Artist = artist
		}
	}

	if tracks, title, artist := parseBeatportJSONLD(doc); len(tracks) > 0 {
		if album.Title == "" && title != "" {
			album.Title = title
		}
		if album.Artist == "" && artist != "" {
			album.Artist = artist
		}
		album.Tracks = tracks
		return &album, nil
	}

	tracks, orderMap := parseBeatportNextData(doc, releaseID)
	if len(tracks) > 0 {
		album.Tracks = tracks
		return &album, nil
	}

	if tracks := parseBeatportDataJSON(doc, releaseID, orderMap); len(tracks) > 0 {
		album.Tracks = tracks
		return &album, nil
	}

	return nil, fmt.Errorf("could not find Beatport track data on page")
}

func getWithUserAgent(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	return http.DefaultClient.Do(req)
}

func parseBeatportMetaTitle(s string) (string, string) {
	s = strings.TrimSpace(strings.ReplaceAll(s, " on Beatport", ""))
	if strings.Contains(s, " by ") {
		parts := strings.SplitN(s, " by ", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(s), ""
}

func extractBeatportReleaseID(url string) int {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) == 0 {
		return 0
	}
	last := parts[len(parts)-1]
	if n, err := strconv.Atoi(last); err == nil {
		return n
	}
	return 0
}

func parseBeatportJSONLD(doc *goquery.Document) ([]AlbumTrack, string, string) {
	var bestTracks []AlbumTrack
	var bestTitle string
	var bestArtist string

	doc.Find("script[type='application/ld+json']").Each(func(_ int, sel *goquery.Selection) {
		raw := strings.TrimSpace(sel.Text())
		if raw == "" {
			return
		}
		var data interface{}
		if err := json.Unmarshal([]byte(raw), &data); err != nil {
			return
		}
		tracks, title, artist := parseLDData(data)
		if len(tracks) > len(bestTracks) {
			bestTracks = tracks
			bestTitle = title
			bestArtist = artist
		}
	})

	return bestTracks, bestTitle, bestArtist
}

func parseLDData(data interface{}) ([]AlbumTrack, string, string) {
	switch v := data.(type) {
	case []interface{}:
		var bestTracks []AlbumTrack
		var bestTitle string
		var bestArtist string
		for _, item := range v {
			tracks, title, artist := parseLDData(item)
			if len(tracks) > len(bestTracks) {
				bestTracks = tracks
				bestTitle = title
				bestArtist = artist
			}
		}
		return bestTracks, bestTitle, bestArtist
	case map[string]interface{}:
		if graph, ok := v["@graph"]; ok {
			return parseLDData(graph)
		}
		typ := getStringFromMap(v, "@type")
		if typ == "MusicAlbum" || typ == "MusicRelease" || typ == "MusicPlaylist" {
			title := getStringFromMap(v, "name", "title")
			artist := parseArtistsField(v["byArtist"])
			if artist == "" {
				artist = parseArtistsField(v["artist"])
			}
			tracks := parseLDTracks(v["track"])
			return tracks, title, artist
		}
	}
	return nil, "", ""
}

func parseLDTracks(value interface{}) []AlbumTrack {
	arr, ok := value.([]interface{})
	if !ok || len(arr) == 0 {
		return nil
	}
	tracks := make([]AlbumTrack, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		title := getStringFromMap(m, "name", "title")
		artist := parseArtistsField(m["byArtist"])
		if artist == "" {
			artist = parseArtistsField(m["artist"])
		}
		trackNum, explicit := extractTrackNumberFromObj(m)
		if trackNum == 0 {
			trackNum = i + 1
		}
		if title != "" {
			tracks = append(tracks, AlbumTrack{
				Title:            title,
				Artist:           artist,
				TrackNum:         trackNum,
				TrackNumExplicit: explicit,
			})
		}
	}
	return tracks
}

func parseBeatportDataJSON(doc *goquery.Document, releaseID int, orderMap map[int]int) []AlbumTrack {
	var tracks []AlbumTrack
	doc.Find("span[data-json]").Each(func(_ int, sel *goquery.Selection) {
		data := strings.TrimSpace(sel.AttrOr("data-json", ""))
		if data == "" {
			return
		}
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(data), &obj); err != nil {
			return
		}
		if releaseID != 0 {
			if objRelease := extractReleaseIDFromObj(obj); objRelease != 0 && objRelease != releaseID {
				return
			}
		}
		trackID := extractTrackIDFromObj(obj)
		title := getStringFromMap(obj, "title", "name")
		artist := parseArtistsField(obj["artists"])
		mixName := getStringFromMap(obj, "mixName", "mix_name")
		if mixName != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(mixName)) {
			title = strings.TrimSpace(title) + " (" + mixName + ")"
		}
		trackNum, explicit := extractTrackNumberFromObj(obj)
		if trackNum == 0 && orderMap != nil && trackID != 0 {
			if n, ok := orderMap[trackID]; ok {
				trackNum = n
				explicit = true
			}
		}
		if trackNum == 0 {
			trackNum = len(tracks) + 1
		}
		if title != "" {
			tracks = append(tracks, AlbumTrack{
				Title:            title,
				Artist:           artist,
				TrackNum:         trackNum,
				TrackNumExplicit: explicit,
				TrackID:          trackID,
			})
		}
	})
	return tracks
}

func parseBeatportNextData(doc *goquery.Document, releaseID int) ([]AlbumTrack, map[int]int) {
	raw := strings.TrimSpace(doc.Find("script#__NEXT_DATA__").Text())
	if raw == "" {
		return nil, nil
	}
	var data interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, nil
	}
	orderMap := findBeatportReleaseTrackOrder(data, releaseID)
	if tracks := extractBeatportTracksFromDehydrated(data, releaseID, orderMap); len(tracks) > 0 {
		return tracks, orderMap
	}
	return findTrackListInJSON(data, releaseID, orderMap), orderMap
}

func findTrackListInJSON(data interface{}, releaseID int, orderMap map[int]int) []AlbumTrack {
	var candidates [][]map[string]interface{}
	collectTrackArrays(data, &candidates)
	if len(candidates) == 0 {
		return nil
	}
	best := scoreTrackCandidate(candidates[0], releaseID, orderMap)
	for _, c := range candidates[1:] {
		cand := scoreTrackCandidate(c, releaseID, orderMap)
		if cand.Score > best.Score {
			best = cand
		}
	}
	ordered := orderTrackObjects(best.Tracks, orderMap)
	tracks := make([]AlbumTrack, 0, len(ordered))
	for i, obj := range ordered {
		trackID := extractTrackIDFromObj(obj)
		title := getStringFromMap(obj, "name", "title")
		artist := parseArtistsField(obj["artists"])
		if artist == "" {
			artist = parseArtistsField(obj["artist"])
		}
		mixName := getStringFromMap(obj, "mixName", "mix_name")
		if mixName != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(mixName)) {
			title = strings.TrimSpace(title) + " (" + mixName + ")"
		}
		trackNum, explicit := extractTrackNumberFromObj(obj)
		if trackNum == 0 && orderMap != nil {
			if trackID != 0 {
				if n, ok := orderMap[trackID]; ok {
					trackNum = n
					explicit = true
				}
			}
		}
		if trackNum == 0 {
			trackNum = i + 1
		}
		if title != "" {
			tracks = append(tracks, AlbumTrack{
				Title:            title,
				Artist:           artist,
				TrackNum:         trackNum,
				TrackNumExplicit: explicit,
				TrackID:          trackID,
			})
		}
	}
	return tracks
}

func extractBeatportTracksFromDehydrated(data interface{}, releaseID int, orderMap map[int]int) []AlbumTrack {
	root, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	props, _ := root["props"].(map[string]interface{})
	pageProps, _ := props["pageProps"].(map[string]interface{})
	dehydrated, _ := pageProps["dehydratedState"].(map[string]interface{})
	queries, _ := dehydrated["queries"].([]interface{})
	if len(queries) == 0 {
		return nil
	}
	for _, q := range queries {
		qm, ok := q.(map[string]interface{})
		if !ok {
			continue
		}
		qk, ok := qm["queryKey"].([]interface{})
		if !ok || len(qk) < 2 {
			continue
		}
		if key, ok := qk[0].(string); !ok || key != "tracks" {
			continue
		}
		params, ok := qk[1].(map[string]interface{})
		if !ok {
			continue
		}
		if releaseID != 0 {
			if rid := parseIntFromAny(params["release_id"], params["releaseId"]); rid != releaseID {
				continue
			}
		}
		state, _ := qm["state"].(map[string]interface{})
		dataNode, _ := state["data"].(map[string]interface{})
		results, _ := dataNode["results"].([]interface{})
		if len(results) == 0 {
			continue
		}
		return buildBeatportTracksFromOrderedResults(results, orderMap)
	}
	return nil
}

func buildBeatportTracksFromOrderedResults(results []interface{}, orderMap map[int]int) []AlbumTrack {
	objs := make([]map[string]interface{}, 0, len(results))
	for _, item := range results {
		if m, ok := item.(map[string]interface{}); ok {
			objs = append(objs, m)
		}
	}
	if len(objs) == 0 {
		return nil
	}
	tracks := make([]AlbumTrack, 0, len(objs))
	for i, obj := range objs {
		trackID := extractTrackIDFromObj(obj)
		title := getStringFromMap(obj, "name", "title")
		artist := parseArtistsField(obj["artists"])
		if artist == "" {
			artist = parseArtistsField(obj["artist"])
		}
		mixName := getStringFromMap(obj, "mixName", "mix_name")
		if mixName != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(mixName)) {
			title = strings.TrimSpace(title) + " (" + mixName + ")"
		}
		trackNum, explicit := extractTrackNumberFromObj(obj)
		if trackNum == 0 && trackID != 0 {
			if n, ok := orderMap[trackID]; ok {
				trackNum = n
				explicit = true
			}
		}
		if trackNum == 0 {
			trackNum = i + 1
		}
		if title != "" {
			tracks = append(tracks, AlbumTrack{
				Title:            title,
				Artist:           artist,
				TrackNum:         trackNum,
				TrackNumExplicit: explicit,
				TrackID:          trackID,
			})
		}
	}
	return tracks
}

type trackListCandidate struct {
	Tracks []map[string]interface{}
	Score  int
}

func scoreTrackCandidate(arr []map[string]interface{}, releaseID int, orderMap map[int]int) trackListCandidate {
	score := len(arr)
	nums := make([]int, 0, len(arr))
	artistCount := 0
	releaseMatch := 0
	releaseSeen := 0
	orderMatches := 0
	for _, obj := range arr {
		if num, _ := extractTrackNumberFromObj(obj); num > 0 {
			nums = append(nums, num)
		}
		if _, ok := obj["artists"]; ok {
			artistCount++
		} else if _, ok := obj["artist"]; ok {
			artistCount++
		}
		if releaseID != 0 {
			if objRelease := extractReleaseIDFromObj(obj); objRelease != 0 {
				releaseSeen++
				if objRelease == releaseID {
					releaseMatch++
				}
			}
		}
		if orderMap != nil {
			if id := extractTrackIDFromObj(obj); id != 0 {
				if _, ok := orderMap[id]; ok {
					orderMatches++
				}
			}
		}
	}
	score += artistCount
	if len(nums) > 0 {
		score += len(nums) * 3
		if isSequentialFromOne(nums, len(arr)) {
			score += 200
		} else if isSequentialFromOne(nums, len(nums)) {
			score += 100
		}
	}
	if releaseID != 0 && releaseSeen > 0 {
		if releaseMatch == releaseSeen && releaseMatch >= len(arr)/2 {
			score += 1000
		} else if releaseMatch > 0 {
			score += releaseMatch * 10
		}
	}
	if orderMatches > 0 {
		score += orderMatches * 5
		if orderMatches >= len(arr)/2 {
			score += 50
		}
	}
	return trackListCandidate{Tracks: arr, Score: score}
}

func isSequentialFromOne(nums []int, expected int) bool {
	if len(nums) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(nums))
	for _, n := range nums {
		if n <= 0 {
			return false
		}
		seen[n] = struct{}{}
	}
	if len(seen) != len(nums) {
		return false
	}
	if expected <= 0 {
		expected = len(nums)
	}
	for i := 1; i <= expected; i++ {
		if _, ok := seen[i]; !ok {
			return false
		}
	}
	return true
}

func orderTrackObjects(arr []map[string]interface{}, orderMap map[int]int) []map[string]interface{} {
	if len(arr) == 0 {
		return arr
	}
	if orderMap != nil && len(orderMap) > 0 {
		ordered := make([]map[string]interface{}, 0, len(arr))
		used := make([]bool, len(arr))
		for i := 1; i <= len(orderMap); i++ {
			for idx, obj := range arr {
				if used[idx] {
					continue
				}
				if id := extractTrackIDFromObj(obj); id != 0 {
					if n, ok := orderMap[id]; ok && n == i {
						ordered = append(ordered, obj)
						used[idx] = true
						break
					}
				}
			}
		}
		if len(ordered) >= len(arr)/2 {
			for idx, obj := range arr {
				if !used[idx] {
					ordered = append(ordered, obj)
				}
			}
			return ordered
		}
	}
	nums := make([]int, 0, len(arr))
	countWithNum := 0
	for _, obj := range arr {
		if n, _ := extractTrackNumberFromObj(obj); n > 0 {
			nums = append(nums, n)
			countWithNum++
		}
	}
	if countWithNum == 0 {
		return arr
	}
	if !isSequentialFromOne(nums, len(arr)) && countWithNum < len(arr)/2 {
		return arr
	}
	ordered := make([]map[string]interface{}, 0, len(arr))
	used := make([]bool, len(arr))
	for i := 1; i <= len(arr); i++ {
		found := false
		for idx, obj := range arr {
			if used[idx] {
				continue
			}
			if n, _ := extractTrackNumberFromObj(obj); n == i {
				ordered = append(ordered, obj)
				used[idx] = true
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if len(ordered) == len(arr) {
		return ordered
	}
	return arr
}

func collectTrackArrays(value interface{}, out *[][]map[string]interface{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		for _, nested := range v {
			collectTrackArrays(nested, out)
		}
	case []interface{}:
		if len(v) > 0 {
			allTracks := true
			arr := make([]map[string]interface{}, 0, len(v))
			for _, item := range v {
				obj, ok := item.(map[string]interface{})
				if !ok || !isTrackLike(obj) {
					allTracks = false
					break
				}
				arr = append(arr, obj)
			}
			if allTracks {
				*out = append(*out, arr)
			}
		}
		for _, nested := range v {
			collectTrackArrays(nested, out)
		}
	}
}

func isTrackLike(obj map[string]interface{}) bool {
	title := getStringFromMap(obj, "name", "title")
	if title == "" {
		return false
	}
	if _, ok := obj["artists"]; ok {
		return true
	}
	if _, ok := obj["artist"]; ok {
		return true
	}
	return false
}

func extractReleaseIDFromObj(obj map[string]interface{}) int {
	if v, ok := obj["releaseId"]; ok {
		return parseIntFromAny(v)
	}
	if v, ok := obj["release_id"]; ok {
		return parseIntFromAny(v)
	}
	if v, ok := obj["release"]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return parseIntFromAny(m["id"], m["releaseId"], m["release_id"])
		}
	}
	return 0
}

func extractTrackIDFromObj(obj map[string]interface{}) int {
	return parseIntFromAny(obj["id"], obj["trackId"], obj["track_id"])
}

func parseBeatportTrackIDFromURL(s string) int {
	if s == "" {
		return 0
	}
	trimmed := strings.TrimRight(s, "/")
	if trimmed == "" {
		return 0
	}
	parts := strings.Split(trimmed, "/")
	last := parts[len(parts)-1]
	if n, err := strconv.Atoi(last); err == nil {
		return n
	}
	return 0
}

func extractTrackIDFromAny(value interface{}) int {
	switch v := value.(type) {
	case string:
		return parseBeatportTrackIDFromURL(v)
	case map[string]interface{}:
		if id := extractTrackIDFromObj(v); id != 0 {
			return id
		}
		if url, ok := v["url"].(string); ok {
			return parseBeatportTrackIDFromURL(url)
		}
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		if v > 0 {
			return int(v)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return 0
}

func buildOrderMapFromTrackList(value interface{}) map[int]int {
	arr, ok := value.([]interface{})
	if !ok || len(arr) == 0 {
		return nil
	}
	order := make(map[int]int, len(arr))
	pos := 1
	for _, item := range arr {
		if id := extractTrackIDFromAny(item); id != 0 {
			if _, exists := order[id]; !exists {
				order[id] = pos
				pos++
			}
		}
	}
	if len(order) == 0 {
		return nil
	}
	return order
}

func findBeatportReleaseTrackOrder(value interface{}, releaseID int) map[int]int {
	switch v := value.(type) {
	case map[string]interface{}:
		if releaseID != 0 {
			if id := parseIntFromAny(v["id"]); id == releaseID {
				if tracksVal, ok := v["tracks"]; ok {
					if order := buildOrderMapFromTrackList(tracksVal); len(order) > 0 {
						return order
					}
				}
			}
		}
		for _, nested := range v {
			if order := findBeatportReleaseTrackOrder(nested, releaseID); len(order) > 0 {
				return order
			}
		}
	case []interface{}:
		for _, item := range v {
			if order := findBeatportReleaseTrackOrder(item, releaseID); len(order) > 0 {
				return order
			}
		}
	}
	return nil
}

func getStringFromMap(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := obj[key]; ok {
			if s, ok := val.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func parseArtistsField(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]interface{}:
		return getStringFromMap(v, "name")
	case []interface{}:
		names := make([]string, 0, len(v))
		for _, item := range v {
			switch t := item.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					names = append(names, strings.TrimSpace(t))
				}
			case map[string]interface{}:
				if name := getStringFromMap(t, "name"); name != "" {
					names = append(names, name)
				}
			}
		}
		return strings.Join(names, ", ")
	}
	return ""
}

func extractTrackNumberFromObj(obj map[string]interface{}) (int, bool) {
	keys := []string{"trackNumber", "track_number", "position", "number", "index", "trackNo"}
	explicit := false
	values := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		if val, ok := obj[key]; ok {
			explicit = true
			values = append(values, val)
		}
	}
	if !explicit {
		return 0, false
	}
	return parseIntFromAny(values...), true
}

func parseIntFromAny(values ...interface{}) int {
	for _, value := range values {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return int(v)
		case float64:
			if v > 0 {
				return int(v)
			}
		case json.Number:
			if n, err := v.Int64(); err == nil {
				return int(n)
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n
			}
		}
	}
	return 0
}
