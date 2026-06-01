package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.pixellab.ai/v2"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type RunOptions struct {
	Env    map[string]string
	Stdout io.Writer
	Stderr io.Writer
}

type EndpointSpec struct {
	Method       string
	Path         string
	RequiredBody []string
	QueryParams  []string
}

type endpointMatch struct {
	Spec        EndpointSpec
	RequestPath string
}

type cliConfig struct {
	Token        string
	BaseURL      string
	Method       string
	JSON         bool
	Out          string
	Wait         bool
	PollInterval time.Duration
	Timeout      time.Duration
	BodyJSON     string
	Quiet        bool
	Flags        map[string][]string
}

type apiError struct {
	Status int
	Body   []byte
}

func (e apiError) Error() string {
	message := strings.TrimSpace(string(e.Body))
	var payload map[string]any
	if json.Unmarshal(e.Body, &payload) == nil {
		if detail, ok := payload["detail"]; ok {
			message = fmt.Sprint(detail)
		} else if errText, ok := payload["error"]; ok {
			message = fmt.Sprint(errText)
		}
	}
	if message == "" {
		message = http.StatusText(e.Status)
	}
	return fmt.Sprintf("API error %d: %s", e.Status, message)
}

func main() {
	os.Exit(Run(context.Background(), os.Args[1:], RunOptions{
		Env:    envMap(os.Environ()),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}))
}

func Run(ctx context.Context, args []string, opts RunOptions) int {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	if opts.Env == nil {
		opts.Env = envMap(os.Environ())
	}

	if len(args) < 1 {
		fmt.Fprintln(stderr, "usage: pxlb <api-route> [flags]")
		return 2
	}
	if args[0] == "--version" || args[0] == "-v" || args[0] == "version" {
		fmt.Fprintf(stdout, "pxlb %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
		return 0
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printHelp(stdout)
		return 0
	}
	pathTemplate := normalizeRouteArg(args[0])

	cfg, err := parseFlags(args[1:], opts.Env)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	match, err := resolveEndpoint(pathTemplate, cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	spec := match.Spec
	method := spec.Method
	if err := rejectPathParamFlags(spec, cfg.Flags); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	requestPath, err := fillPathParams(match.RequestPath, cfg.Flags)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	body, err := buildBody(spec, cfg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if cfg.BodyJSON == "" {
		if err := validateRequired(spec, body); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}

	query := buildQuery(spec, cfg.Flags)
	responseBody, contentType, err := executeRequest(ctx, method, cfg.BaseURL, requestPath, query, cfg.Token, body)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if cfg.Out != "" && !isJSONContent(contentType) {
		if err := writeBytesOutput(cfg.Out, responseBody); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	jsonPayload, err := decodeJSON(responseBody)
	if err != nil {
		if cfg.Out != "" {
			if err := writeBytesOutput(cfg.Out, responseBody); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		fmt.Fprintln(stdout, string(responseBody))
		return 0
	}

	if cfg.Wait {
		jsonPayload, err = waitForJob(ctx, cfg, jsonPayload)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if cfg.Out != "" {
		if err := writeImagesFromPayload(cfg.Out, jsonPayload); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	if !cfg.Quiet {
		data, _ := json.MarshalIndent(jsonPayload, "", "  ")
		fmt.Fprintln(stdout, string(data))
	}
	return 0
}

func EndpointSpecs() []EndpointSpec {
	return []EndpointSpec{
		{Method: "DELETE", Path: "/characters/{character_id}"},
		{Method: "DELETE", Path: "/objects/{object_id}"},
		{Method: "GET", Path: "/background-jobs/{job_id}"},
		{Method: "GET", Path: "/balance"},
		{Method: "GET", Path: "/characters", QueryParams: []string{"limit", "offset"}},
		{Method: "GET", Path: "/characters/{character_id}"},
		{Method: "GET", Path: "/characters/{character_id}/zip"},
		{Method: "GET", Path: "/isometric-tiles", QueryParams: []string{"limit", "offset"}},
		{Method: "GET", Path: "/isometric-tiles/{tile_id}"},
		{Method: "GET", Path: "/llms.txt"},
		{Method: "GET", Path: "/objects", QueryParams: []string{"limit", "offset"}},
		{Method: "GET", Path: "/objects/{object_id}"},
		{Method: "GET", Path: "/tiles-pro/{tile_id}"},
		{Method: "GET", Path: "/tilesets", QueryParams: []string{"limit", "offset"}},
		{Method: "GET", Path: "/tilesets/{tileset_id}"},
		{Method: "PATCH", Path: "/characters/{character_id}/tags", RequiredBody: []string{"tags"}},
		{Method: "PATCH", Path: "/objects/{object_id}/tags", RequiredBody: []string{"tags"}},
		{Method: "POST", Path: "/animate-character", RequiredBody: []string{"character_id"}},
		{Method: "POST", Path: "/animate-with-skeleton", RequiredBody: []string{"image_size", "reference_image"}},
		{Method: "POST", Path: "/animate-with-text", RequiredBody: []string{"image_size", "description", "action", "reference_image"}},
		{Method: "POST", Path: "/animate-with-text-v2", RequiredBody: []string{"reference_image", "reference_image_size", "action", "image_size"}},
		{Method: "POST", Path: "/animate-with-text-v3", RequiredBody: []string{"first_frame", "action"}},
		{Method: "POST", Path: "/characters/animations", RequiredBody: []string{"character_id"}},
		{Method: "POST", Path: "/create-1-direction-object", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/create-8-direction-object", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/create-character-pro", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-character-state", RequiredBody: []string{"character_id", "edit_description"}},
		{Method: "POST", Path: "/create-character-v3", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/create-character-with-4-directions", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-character-with-8-directions", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-image-bitforge", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-image-pixen", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-image-pixflux", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-isometric-tile", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/create-tiles-pro", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/create-tileset", RequiredBody: []string{"lower_description", "upper_description"}},
		{Method: "POST", Path: "/create-tileset-sidescroller", RequiredBody: []string{"lower_description"}},
		{Method: "POST", Path: "/edit-animation-v2", RequiredBody: []string{"description", "frames", "image_size"}},
		{Method: "POST", Path: "/edit-image", RequiredBody: []string{"image", "image_size", "description", "width", "height"}},
		{Method: "POST", Path: "/edit-images-v2", RequiredBody: []string{"edit_images", "image_size"}},
		{Method: "POST", Path: "/enhance-animation-v3-prompt", RequiredBody: []string{"first_frame", "action"}},
		{Method: "POST", Path: "/enhance-character-v3-prompt", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/enhance-pixen-prompt", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/estimate-skeleton"},
		{Method: "POST", Path: "/generate-8-rotations-v2", RequiredBody: []string{"image_size"}},
		{Method: "POST", Path: "/generate-8-rotations-v3", RequiredBody: []string{"first_frame"}},
		{Method: "POST", Path: "/generate-image-v2", RequiredBody: []string{"description", "image_size"}},
		{Method: "POST", Path: "/generate-ui-v2", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/generate-with-style-v2", RequiredBody: []string{"style_images", "description", "image_size"}},
		{Method: "POST", Path: "/image-to-pixelart", RequiredBody: []string{"image", "image_size", "output_size"}},
		{Method: "POST", Path: "/inpaint", RequiredBody: []string{"description", "image_size", "inpainting_image", "mask_image"}},
		{Method: "POST", Path: "/inpaint-v3", RequiredBody: []string{"description", "inpainting_image", "mask_image"}},
		{Method: "POST", Path: "/interpolation-v2", RequiredBody: []string{"start_image", "end_image", "action", "image_size"}},
		{Method: "POST", Path: "/map-objects", RequiredBody: []string{"description"}},
		{Method: "POST", Path: "/objects/{object_id}/animations"},
		{Method: "POST", Path: "/objects/{object_id}/dismiss-review"},
		{Method: "POST", Path: "/objects/{object_id}/select-frames", RequiredBody: []string{"indices"}},
		{Method: "POST", Path: "/objects/{object_id}/states", RequiredBody: []string{"edit_description"}},
		{Method: "POST", Path: "/remove-background", RequiredBody: []string{"image", "image_size"}},
		{Method: "POST", Path: "/resize", RequiredBody: []string{"description", "reference_image", "reference_image_size", "target_size"}},
		{Method: "POST", Path: "/rotate", RequiredBody: []string{"image_size", "from_image"}},
		{Method: "POST", Path: "/tilesets", RequiredBody: []string{"lower_description", "upper_description"}},
		{Method: "POST", Path: "/tilesets-sidescroller", RequiredBody: []string{"lower_description"}},
		{Method: "POST", Path: "/transfer-outfit-v2", RequiredBody: []string{"reference_image", "frames", "image_size"}},
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "pxlb - PixelLab v2 API CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  pxlb <api-route> [flags]")
	fmt.Fprintln(w, "  pxlb --version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  --token <key>            PixelLab API key; overrides PIXELLAB_API_KEY and .env.")
	fmt.Fprintln(w, "  --base-url <url>         API base URL; defaults to https://api.pixellab.ai/v2.")
	fmt.Fprintln(w, "  --http-method <method>   Override inferred HTTP method for colliding routes.")
	fmt.Fprintln(w, "  --body-json <json|file>  Send a raw JSON request body.")
	fmt.Fprintln(w, "  --wait                   Poll returned background jobs until completion.")
	fmt.Fprintln(w, "  --out <path>             Save returned binary or base64 image outputs.")
	fmt.Fprintln(w, "  --quiet                  Suppress normal JSON output.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Routes:")

	entries := helpEntries()
	width := 0
	for _, entry := range entries {
		if len(entry.Command) > width {
			width = len(entry.Command)
		}
	}
	for _, entry := range entries {
		fmt.Fprintf(w, "  %-*s  %s\n", width, entry.Command, entry.Description)
	}
}

type helpEntry struct {
	Command     string
	Description string
}

func helpEntries() []helpEntry {
	entries := make([]helpEntry, 0, len(EndpointSpecs()))
	for _, spec := range EndpointSpecs() {
		entries = append(entries, helpEntry{
			Command:     helpCommand(spec),
			Description: routeDescription(spec),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Command == entries[j].Command {
			return entries[i].Description < entries[j].Description
		}
		return entries[i].Command < entries[j].Command
	})
	return entries
}

func helpCommand(spec EndpointSpec) string {
	route := displayRoute(spec.Path)
	if spec.Method == "DELETE" {
		return route + " --http-method delete"
	}
	if spec.Path == "/tilesets" && spec.Method == "POST" {
		return route + " --lower-description ... --upper-description ..."
	}
	return route
}

func displayRoute(path string) string {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		if isPlaceholderSegment(part) {
			parts[i] = "<" + part[1:len(part)-1] + ">"
		}
	}
	return strings.Join(parts, "/")
}

func routeDescription(spec EndpointSpec) string {
	switch spec.Method + " " + spec.Path {
	case "DELETE /characters/{character_id}":
		return "Delete a character and its associated data."
	case "DELETE /objects/{object_id}":
		return "Delete an object and its associated rotations, animations, and tags."
	case "GET /background-jobs/{job_id}":
		return "Check the status and result of an asynchronous background job."
	case "GET /balance":
		return "Show account credits and subscription generation balance."
	case "GET /characters":
		return "List characters with pagination."
	case "GET /characters/{character_id}":
		return "Show character details, rotations, animations, and download links."
	case "GET /characters/{character_id}/zip":
		return "Download a character ZIP archive."
	case "GET /isometric-tiles":
		return "List generated isometric tiles."
	case "GET /isometric-tiles/{tile_id}":
		return "Show an isometric tile or its generation status."
	case "GET /llms.txt":
		return "Print PixelLab's LLM-oriented API documentation."
	case "GET /objects":
		return "List objects with pagination."
	case "GET /objects/{object_id}":
		return "Show object details, rotations, animations, and download links."
	case "GET /tiles-pro/{tile_id}":
		return "Show generated pro tile data and storage URLs."
	case "GET /tilesets":
		return "List generated top-down tilesets."
	case "GET /tilesets/{tileset_id}":
		return "Show a tileset, download links, and generation status."
	case "PATCH /characters/{character_id}/tags":
		return "Replace a character's tags."
	case "PATCH /objects/{object_id}/tags":
		return "Replace an object's tags."
	case "POST /animate-character":
		return "Queue a character animation from a template or text description."
	case "POST /animate-with-skeleton":
		return "Generate animation frames from a reference image and skeleton keypoints."
	case "POST /animate-with-text":
		return "Generate legacy text-guided animation frames from a reference image."
	case "POST /animate-with-text-v2":
		return "Queue pro text-guided animation from a reference image."
	case "POST /animate-with-text-v3":
		return "Generate v3 text-guided animation from first and optional last frames."
	case "POST /characters/animations":
		return "Queue a character animation using the character animation API."
	case "POST /create-1-direction-object":
		return "Create one-direction pixel art objects, optionally as review candidates."
	case "POST /create-8-direction-object":
		return "Create an object rendered from eight directions."
	case "POST /create-character-pro":
		return "Create a pro-mode character."
	case "POST /create-character-state":
		return "Create an edited state of an existing character."
	case "POST /create-character-v3":
		return "Create a v3 character."
	case "POST /create-character-with-4-directions":
		return "Create a four-direction character."
	case "POST /create-character-with-8-directions":
		return "Create an eight-direction character."
	case "POST /create-image-bitforge":
		return "Create pixel art with the Bitforge image model."
	case "POST /create-image-pixen":
		return "Create pixel art with the Pixen image model."
	case "POST /create-image-pixflux":
		return "Create pixel art with the Pixflux image model."
	case "POST /create-isometric-tile":
		return "Create an isometric pixel art tile."
	case "POST /create-tiles-pro":
		return "Create multiple pro tile variations."
	case "POST /create-tileset":
		return "Create a top-down Wang tileset with terrain transitions."
	case "POST /create-tileset-sidescroller":
		return "Create a sidescroller platformer tileset."
	case "POST /edit-animation-v2":
		return "Edit existing animation frames with a text instruction."
	case "POST /edit-image":
		return "Edit a single image with text guidance."
	case "POST /edit-images-v2":
		return "Edit multiple images with text guidance."
	case "POST /enhance-animation-v3-prompt":
		return "Enhance an animation action prompt for v3 generation."
	case "POST /enhance-character-v3-prompt":
		return "Enhance a character creation prompt."
	case "POST /enhance-pixen-prompt":
		return "Enhance an image generation prompt."
	case "POST /estimate-skeleton":
		return "Estimate skeleton keypoints from an image."
	case "POST /generate-8-rotations-v2":
		return "Queue pro generation of eight rotations for an image."
	case "POST /generate-8-rotations-v3":
		return "Generate eight rotations from a first frame."
	case "POST /generate-image-v2":
		return "Queue pro image generation."
	case "POST /generate-ui-v2":
		return "Queue UI asset generation."
	case "POST /generate-with-style-v2":
		return "Queue image generation matching style reference images."
	case "POST /image-to-pixelart":
		return "Convert an image to pixel art."
	case "POST /inpaint":
		return "Inpaint an image region using a mask."
	case "POST /inpaint-v3":
		return "Inpaint an image region with the v3 inpainting model."
	case "POST /interpolation-v2":
		return "Interpolate animation frames between start and end images."
	case "POST /map-objects":
		return "Create a map object, optionally style-matched to a background."
	case "POST /objects/{object_id}/animations":
		return "Queue an animation for an existing object."
	case "POST /objects/{object_id}/dismiss-review":
		return "Discard all review candidates for an object."
	case "POST /objects/{object_id}/select-frames":
		return "Promote selected review candidates into completed objects."
	case "POST /objects/{object_id}/states":
		return "Create an edited state of an existing object."
	case "POST /remove-background":
		return "Remove the background from an image."
	case "POST /resize":
		return "Resize and regenerate an image to a target size."
	case "POST /rotate":
		return "Rotate an image from one direction to another."
	case "POST /tilesets":
		return "Create a top-down Wang tileset with terrain transitions."
	case "POST /tilesets-sidescroller":
		return "Create a sidescroller platformer tileset."
	case "POST /transfer-outfit-v2":
		return "Transfer outfit or appearance from a reference onto frames."
	default:
		return "Call this PixelLab API route."
	}
}

func findEndpoint(method, path string) (EndpointSpec, bool) {
	for _, spec := range EndpointSpecs() {
		if spec.Method == method && spec.Path == path {
			return spec, true
		}
	}
	return EndpointSpec{}, false
}

func normalizeRouteArg(route string) string {
	if strings.HasPrefix(route, "/") {
		return route
	}
	return "/" + route
}

func resolveEndpoint(path string, cfg cliConfig) (endpointMatch, error) {
	matches := endpointsForPath(path)
	if cfg.Method != "" {
		for _, match := range matches {
			if match.Spec.Method == cfg.Method {
				return match, nil
			}
		}
		if len(matches) == 0 {
			return endpointMatch{}, fmt.Errorf("unknown endpoint: %s", path)
		}
		return endpointMatch{}, fmt.Errorf("endpoint %s does not support --http-method %s; available methods: %s", path, strings.ToLower(cfg.Method), availableMethods(matches))
	}
	if len(matches) == 0 {
		return endpointMatch{}, fmt.Errorf("unknown endpoint: %s", path)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if bodyInputPresent(cfg, matches) {
		var bodyMatches []endpointMatch
		for _, match := range matches {
			if match.Spec.Method != "GET" && match.Spec.Method != "DELETE" {
				bodyMatches = append(bodyMatches, match)
			}
		}
		if len(bodyMatches) == 1 {
			return bodyMatches[0], nil
		}
	}
	for _, match := range matches {
		if match.Spec.Method == "GET" {
			return match, nil
		}
	}
	return endpointMatch{}, fmt.Errorf("ambiguous endpoint: %s; use --http-method with one of: %s", path, availableMethods(matches))
}

func endpointsForPath(path string) []endpointMatch {
	var matches []endpointMatch
	for _, spec := range EndpointSpecs() {
		requestPath, ok := matchEndpointPath(spec.Path, path)
		if ok {
			matches = append(matches, endpointMatch{Spec: spec, RequestPath: requestPath})
		}
	}
	return matches
}

func matchEndpointPath(template string, path string) (string, bool) {
	templateParts := strings.Split(strings.Trim(template, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")
	if len(templateParts) != len(pathParts) {
		return "", false
	}
	requestParts := make([]string, len(pathParts))
	for i := range templateParts {
		if isUserPlaceholderSegment(pathParts[i]) {
			return "", false
		}
		if isPlaceholderSegment(templateParts[i]) {
			if pathParts[i] == "" {
				return "", false
			}
			requestParts[i] = url.PathEscape(pathParts[i])
			continue
		}
		if templateParts[i] != pathParts[i] {
			return "", false
		}
		requestParts[i] = templateParts[i]
	}
	return "/" + strings.Join(requestParts, "/"), true
}

func isPlaceholderSegment(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") && len(segment) > 2
}

func isUserPlaceholderSegment(segment string) bool {
	return isPlaceholderSegment(segment) || (strings.HasPrefix(segment, "<") && strings.HasSuffix(segment, ">") && len(segment) > 2)
}

func bodyInputPresent(cfg cliConfig, matches []endpointMatch) bool {
	if cfg.BodyJSON != "" {
		return true
	}
	queryNames := map[string]bool{}
	for _, match := range matches {
		for _, name := range match.Spec.QueryParams {
			queryNames[snakeToKebab(name)] = true
		}
	}
	for flagName := range cfg.Flags {
		if queryNames[flagName] {
			continue
		}
		return true
	}
	return false
}

func availableMethods(matches []endpointMatch) string {
	methods := make([]string, 0, len(matches))
	for _, match := range matches {
		methods = append(methods, strings.ToLower(match.Spec.Method))
	}
	return strings.Join(methods, ", ")
}

func rejectPathParamFlags(spec EndpointSpec, flags map[string][]string) error {
	for _, name := range pathParamNames(spec.Path) {
		flagName := snakeToKebab(name)
		if len(flags[flagName]) > 0 {
			return fmt.Errorf("route parameter %s belongs in the route, for example: %s", flagName, displayRoute(spec.Path))
		}
	}
	return nil
}

func pathParamNames(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if isPlaceholderSegment(part) {
			names = append(names, part[1:len(part)-1])
		}
	}
	return names
}

func parseFlags(args []string, env map[string]string) (cliConfig, error) {
	cfg := cliConfig{
		Token:        tokenFromEnv(env),
		BaseURL:      defaultBaseURL,
		PollInterval: 5 * time.Second,
		Flags:        map[string][]string{},
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			return cfg, fmt.Errorf("unexpected positional argument: %s", arg)
		}
		name := strings.TrimPrefix(arg, "--")
		value := "true"
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			value = args[i+1]
			i++
		}
		switch name {
		case "token":
			cfg.Token = value
		case "base-url":
			cfg.BaseURL = value
		case "http-method":
			method := strings.ToUpper(value)
			switch method {
			case "GET", "POST", "PATCH", "DELETE":
				cfg.Method = method
			default:
				return cfg, fmt.Errorf("invalid --http-method: %s", value)
			}
		case "json":
			cfg.JSON = parseBool(value)
		case "out":
			cfg.Out = value
		case "wait":
			cfg.Wait = parseBool(value)
		case "poll-interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return cfg, fmt.Errorf("invalid --poll-interval: %w", err)
			}
			cfg.PollInterval = d
		case "timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return cfg, fmt.Errorf("invalid --timeout: %w", err)
			}
			cfg.Timeout = d
		case "body-json":
			cfg.BodyJSON = value
		case "quiet":
			cfg.Quiet = parseBool(value)
		default:
			cfg.Flags[name] = append(cfg.Flags[name], value)
		}
	}
	return cfg, nil
}

func fillPathParams(path string, flags map[string][]string) (string, error) {
	for {
		start := strings.Index(path, "{")
		if start == -1 {
			return path, nil
		}
		end := strings.Index(path[start:], "}")
		if end == -1 {
			return "", fmt.Errorf("invalid path template: %s", path)
		}
		name := path[start+1 : start+end]
		flagName := snakeToKebab(name)
		values := flags[flagName]
		if len(values) == 0 {
			return "", fmt.Errorf("missing --%s", flagName)
		}
		path = path[:start] + url.PathEscape(values[len(values)-1]) + path[start+end+1:]
	}
}

func buildQuery(spec EndpointSpec, flags map[string][]string) url.Values {
	query := url.Values{}
	for _, name := range spec.QueryParams {
		flagName := snakeToKebab(name)
		if values := flags[flagName]; len(values) > 0 {
			query.Set(name, values[len(values)-1])
		}
	}
	return query
}

func buildBody(spec EndpointSpec, cfg cliConfig) (map[string]any, error) {
	if cfg.BodyJSON != "" {
		return readJSONBody(cfg.BodyJSON)
	}
	if spec.Method == "GET" || spec.Method == "DELETE" {
		return nil, nil
	}
	body := map[string]any{}
	queryNames := map[string]bool{}
	for _, name := range spec.QueryParams {
		queryNames[snakeToKebab(name)] = true
	}
	for flagName, values := range cfg.Flags {
		if queryNames[flagName] || isPathFlag(flagName, spec.Path) {
			continue
		}
		if err := applyFlagToBody(spec, body, flagName, values); err != nil {
			return nil, err
		}
	}
	return body, nil
}

func applyFlagToBody(spec EndpointSpec, body map[string]any, flagName string, values []string) error {
	if len(values) == 0 {
		return nil
	}
	switch flagName {
	case "tag":
		body["tags"] = stringArray(values)
		return nil
	case "index":
		indices := make([]int, 0, len(values))
		for _, value := range values {
			index, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid --index: %w", err)
			}
			indices = append(indices, index)
		}
		body["indices"] = indices
		return nil
	case "item-description":
		body["item_descriptions"] = stringArray(values)
		return nil
	case "directions":
		body["directions"] = splitCSV(values[len(values)-1])
		return nil
	case "style-image":
		if spec.Path == "/generate-with-style-v2" || spec.Path == "/create-1-direction-object" {
			images := make([]any, 0, len(values))
			for _, value := range values {
				image, err := fileToImage(value)
				if err != nil {
					return err
				}
				images = append(images, image)
			}
			body["style_images"] = images
			return nil
		}
	}

	jsonName := kebabToSnake(flagName)
	if strings.HasSuffix(flagName, "-json") {
		jsonName = kebabToSnake(strings.TrimSuffix(flagName, "-json"))
		var value any
		if err := json.Unmarshal([]byte(readMaybeFile(values[len(values)-1])), &value); err != nil {
			return fmt.Errorf("invalid --%s JSON: %w", flagName, err)
		}
		body[jsonName] = value
		return nil
	}

	if isImageFlag(flagName) || strings.HasSuffix(flagName, "-frame") {
		image, err := fileToImage(values[len(values)-1])
		if err != nil {
			return err
		}
		body[jsonName] = image
		return nil
	}

	if isRepeatedBodyFlag(flagName) || len(values) > 1 {
		var array []any
		for _, value := range values {
			array = append(array, inferValue(value))
		}
		body[jsonName] = array
		return nil
	}

	value := values[len(values)-1]
	if isSizeFlag(flagName) {
		parsed, err := parseSizeOrNumber(value)
		if err != nil {
			return fmt.Errorf("invalid --%s: %w", flagName, err)
		}
		body[jsonName] = parsed
		return nil
	}
	body[jsonName] = inferValue(value)
	return nil
}

func validateRequired(spec EndpointSpec, body map[string]any) error {
	if len(spec.RequiredBody) == 0 {
		return nil
	}
	for _, name := range spec.RequiredBody {
		if body == nil || body[name] == nil {
			return fmt.Errorf("missing --%s", snakeToKebab(name))
		}
	}
	return nil
}

func executeRequest(ctx context.Context, method, baseURL, path string, query url.Values, token string, body map[string]any) ([]byte, string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, "", err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", apiError{Status: resp.StatusCode, Body: data}
	}
	return data, resp.Header.Get("Content-Type"), nil
}

func waitForJob(ctx context.Context, cfg cliConfig, initial map[string]any) (map[string]any, error) {
	jobID := extractJobID(initial)
	if jobID == "" {
		return initial, nil
	}
	deadline := time.Time{}
	if cfg.Timeout > 0 {
		deadline = time.Now().Add(cfg.Timeout)
	}
	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for job %s", jobID)
		}
		data, _, err := executeRequest(ctx, http.MethodGet, cfg.BaseURL, "/background-jobs/"+url.PathEscape(jobID), nil, cfg.Token, nil)
		if err != nil {
			return nil, err
		}
		payload, err := decodeJSON(data)
		if err != nil {
			return nil, err
		}
		status := strings.ToLower(fmt.Sprint(payload["status"]))
		switch status {
		case "completed", "complete", "succeeded", "success":
			if last, ok := payload["last_response"].(map[string]any); ok {
				return last, nil
			}
			return payload, nil
		case "failed", "error":
			return nil, fmt.Errorf("job %s failed: %v", jobID, payload)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(cfg.PollInterval):
		}
	}
}

func writeImagesFromPayload(out string, payload map[string]any) error {
	images := collectImages(payload)
	if len(images) == 0 {
		return nil
	}
	if len(images) == 1 && filepath.Ext(out) != "" {
		return writeDecodedImage(out, images[0], 0)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for i, image := range images {
		format := fmt.Sprint(image["format"])
		if format == "" || format == "<nil>" {
			format = "png"
		}
		path := filepath.Join(out, fmt.Sprintf("image_%02d.%s", i, strings.TrimPrefix(format, ".")))
		if err := writeDecodedImage(path, image, i); err != nil {
			return err
		}
	}
	return nil
}

func writeDecodedImage(path string, image map[string]any, _ int) error {
	raw, ok := image["base64"].(string)
	if !ok || raw == "" {
		return nil
	}
	if comma := strings.Index(raw, ","); comma != -1 && strings.Contains(raw[:comma], "base64") {
		raw = raw[comma+1:]
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return err
	}
	return writeBytesOutput(path, data)
}

func writeBytesOutput(out string, data []byte) error {
	if filepath.Ext(out) == "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			return err
		}
		out = filepath.Join(out, "response.bin")
	} else if dir := filepath.Dir(out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(out, data, 0o600)
}

func collectImages(payload any) []map[string]any {
	var images []map[string]any
	switch value := payload.(type) {
	case map[string]any:
		if base64Value, ok := value["base64"].(string); ok && base64Value != "" {
			images = append(images, value)
		}
		for _, child := range value {
			images = append(images, collectImages(child)...)
		}
	case []any:
		for _, child := range value {
			images = append(images, collectImages(child)...)
		}
	}
	return images
}

func extractJobID(payload map[string]any) string {
	for _, key := range []string{"background_job_id", "job_id"} {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	if value, ok := payload["id"].(string); ok && strings.Contains(strings.ToLower(fmt.Sprint(payload["status"])), "process") {
		return value
	}
	for _, value := range payload {
		if jobID := extractNestedJobID(value); jobID != "" {
			return jobID
		}
	}
	return ""
}

func extractNestedJobID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		return extractJobID(typed)
	case []any:
		for _, item := range typed {
			if jobID := extractNestedJobID(item); jobID != "" {
				return jobID
			}
		}
	}
	return ""
}

func decodeJSON(data []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func fileToImage(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if format == "jpg" {
		format = "jpeg"
	}
	if format == "" {
		format = "png"
	}
	return map[string]any{
		"type":   "base64",
		"base64": base64.StdEncoding.EncodeToString(data),
		"format": format,
	}, nil
}

func readJSONBody(value string) (map[string]any, error) {
	raw := readMaybeFile(value)
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("invalid --body-json: %w", err)
	}
	return body, nil
}

func readMaybeFile(value string) string {
	if strings.HasPrefix(strings.TrimSpace(value), "{") || strings.HasPrefix(strings.TrimSpace(value), "[") {
		return value
	}
	data, err := os.ReadFile(value)
	if err == nil {
		return string(data)
	}
	return value
}

func parseSizeOrNumber(value string) (any, error) {
	if strings.Contains(value, "x") {
		parts := strings.Split(value, "x")
		if len(parts) != 2 {
			return nil, errors.New("expected WIDTHxHEIGHT")
		}
		width, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, err
		}
		height, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		return map[string]any{"width": width, "height": height}, nil
	}
	return inferValue(value), nil
}

func inferValue(value string) any {
	if value == "true" || value == "false" {
		return parseBool(value)
	}
	if integer, err := strconv.Atoi(value); err == nil {
		return integer
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil && strings.Contains(value, ".") {
		return number
	}
	return value
}

func isPathFlag(flagName string, path string) bool {
	return strings.Contains(path, "{"+kebabToSnake(flagName)+"}")
}

func isImageFlag(flagName string) bool {
	return flagName == "image" || strings.HasSuffix(flagName, "-image") || strings.HasSuffix(flagName, "-images") || flagName == "from-image"
}

func isSizeFlag(flagName string) bool {
	return flagName == "size" || strings.HasSuffix(flagName, "-size")
}

func isRepeatedBodyFlag(flagName string) bool {
	switch flagName {
	case "style-image", "reference-image", "item-description":
		return true
	}
	return false
}

func isJSONContent(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json") || contentType == ""
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return true
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func stringArray(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func kebabToSnake(value string) string {
	return strings.ReplaceAll(value, "-", "_")
}

func snakeToKebab(value string) string {
	return strings.ReplaceAll(value, "_", "-")
}

func envMap(env []string) map[string]string {
	out := map[string]string{}
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func tokenFromEnv(env map[string]string) string {
	if token := env["PIXELLAB_API_KEY"]; token != "" {
		return token
	}
	dotenv := loadDotEnv(".env")
	return dotenv["PIXELLAB_API_KEY"]
}

func loadDotEnv(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		if key != "" {
			values[key] = value
		}
	}
	return values
}
