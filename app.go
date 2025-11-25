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
	reOriginal := regexp.MustCompile(`^(?i)(?P<artist>.+?)\s+-\s+(?P<album>.+?)\s+-\s+(?P<track>\d+)\s+(?P<title>.+?)(?:\s+\((?P<bpm>\d+)\))?\.(?P<ext>flac|mp3|wav|aiff)$`)

	for _, localTrack := range localTracks {
		matches := reOriginal.FindStringSubmatch(localTrack.OriginalName)
		track := MatchedTrack{
			LocalPath:       localTrack.Path,
			OriginalName:    localTrack.OriginalName,
			ProposedNewName: localTrack.OriginalName,
			Confidence:      0,
			Status:          "No Match",
		}

		if matches != nil {
			result := make(map[string]string)
			for i, name := range reOriginal.SubexpNames() {
				if i != 0 && name != "" {
					result[name] = matches[i]
				}
			}

			var newNameBuilder strings.Builder
			newNameBuilder.WriteString(fmt.Sprintf("%02s", result["track"]))
			newNameBuilder.WriteString(". ")

			if format == "Track. Title" {
				// Just Track. Title
				newNameBuilder.WriteString(result["title"])
			} else {
				// Default: Track. Artist - Title
				newNameBuilder.WriteString(result["artist"])
				newNameBuilder.WriteString(" - ")
				newNameBuilder.WriteString(result["title"])
			}

			if bpm, ok := result["bpm"]; ok && bpm != "" {
				newNameBuilder.WriteString(" (")
				newNameBuilder.WriteString(bpm)
				newNameBuilder.WriteString(")")
			}
			newNameBuilder.WriteString(".")
			newNameBuilder.WriteString(result["ext"])

			track.ProposedNewName = newNameBuilder.String()
			track.Confidence = 1.0
			track.Status = "Matched"
		}
		matchedTracks = append(matchedTracks, track)
	}
	return matchedTracks, nil
}

func (a *App) FetchAndMatchTracks(url string, localTracks []LocalTrack) ([]MatchedTrack, error) {
	album, err := a.fetchBandcampData(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch or parse bandcamp data: %w", err)
	}

	var matchedTracks []MatchedTrack
	sm := metrics.NewSorensenDice()
	sm.CaseSensitive = false

	availableLocalTracks := make([]LocalTrack, len(localTracks))
	copy(availableLocalTracks, localTracks)

	log.Printf("Bandcamp URL: %s", url)
	log.Printf("Album Artist from Bandcamp: %s", album.Artist)
	lowerAlbumArtist := strings.ToLower(album.Artist)
	isVA := strings.Contains(lowerAlbumArtist, "various") || strings.Contains(lowerAlbumArtist, "v.a.") || strings.Contains(lowerAlbumArtist, "va ") || strings.Contains(lowerAlbumArtist, "various artists") || strings.Contains(url, "/va-")
	log.Printf("Is VA Album (calculated): %t", isVA)

	for _, bcTrack := range album.TrackInfo {
		log.Printf("Processing Bandcamp Track: %s (Num: %d)", bcTrack.Title, bcTrack.TrackNum)

		if len(availableLocalTracks) == 0 {
			break
		}

		var bestMatchIndex = -1
		var bestMatchRating float64 = -1.0

		for i, local := range availableLocalTracks {
			// **FIXED**: Use the full original name (without extension) for comparison.
			localNameToCompare := strings.TrimSuffix(local.OriginalName, filepath.Ext(local.OriginalName))

			// Calculate similarity based on filename
			ratingFilename := strutil.Similarity(bcTrack.Title, localNameToCompare, sm)

			// Calculate similarity based on tags (if available)
			ratingTags := 0.0
			if local.TagTitle != "" {
				ratingTags = strutil.Similarity(bcTrack.Title, local.TagTitle, sm)
			}

			// Use the higher of the two ratings
			rating := ratingFilename
			if ratingTags > rating {
				rating = ratingTags
				log.Printf("  Higher match found using tags for '%s': %f", local.OriginalName, rating)
			}

			log.Printf("  Comparing Bandcamp '%s' with Local '%s' (Tags: '%s' - '%s')", bcTrack.Title, localNameToCompare, local.TagArtist, local.TagTitle)
			log.Printf("  Similarity Rating: %f", rating)

			if rating > bestMatchRating {
				bestMatchRating = rating
				bestMatchIndex = i
			}
		}

		if bestMatchIndex != -1 {
			matchedLocalTrack := availableLocalTracks[bestMatchIndex]
			ext := filepath.Ext(matchedLocalTrack.OriginalName)

			// 1. Clean the title to handle cases where the track number is duplicated by Bandcamp
			cleanedTitle := bcTrack.Title
			prefixes_to_check := []string{
				fmt.Sprintf("%d. ", bcTrack.TrackNum),
				fmt.Sprintf("%02d. ", bcTrack.TrackNum),
				fmt.Sprintf("%d - ", bcTrack.TrackNum),
				fmt.Sprintf("%02d - ", bcTrack.TrackNum),
			}
			for _, prefix := range prefixes_to_check {
				if strings.HasPrefix(cleanedTitle, prefix) {
					cleanedTitle = strings.TrimPrefix(cleanedTitle, prefix)
					break // Stop after finding the first match
				}
			}

			// 2. Now apply the logic to construct the name
			var proposedName string
			albumTitleParts := strings.SplitN(album.Current.Title, " - ", 2)
			var albumArtistFromTitle string
			if len(albumTitleParts) > 1 {
				albumArtistFromTitle = albumTitleParts[0]
			}

			// Use the cleaned title for the check
			if strings.Contains(cleanedTitle, "-") {
				// Title is likely "Artist - Title", so use it as is.
				proposedName = fmt.Sprintf("%02d. %s%s", bcTrack.TrackNum, cleanedTitle, ext)
			} else if albumArtistFromTitle != "" {
				// Title does not contain artist, so prepend the artist from the album title.
				proposedName = fmt.Sprintf("%02d. %s - %s%s", bcTrack.TrackNum, albumArtistFromTitle, cleanedTitle, ext)
			} else {
				// Fallback: can't find artist anywhere, just use the cleaned title.
				proposedName = fmt.Sprintf("%02d. %s%s", bcTrack.TrackNum, cleanedTitle, ext)
			}

			matchedTracks = append(matchedTracks, MatchedTrack{
				LocalPath:       matchedLocalTrack.Path,
				OriginalName:    matchedLocalTrack.OriginalName,
				ProposedNewName: proposedName,
				Confidence:      bestMatchRating,
				Status:          "Bandcamp Match",
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

func (a *App) fetchBandcampData(url string) (*BandcampAlbum, error) {
	res, err := http.Get(url)
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
