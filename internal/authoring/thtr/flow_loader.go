package thtr

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alex-poliushkin/theater"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
)

func LoadFlowFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	result, err := LoadFlowFileDetailed(path, matchers)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return result.Spec, nil
}

type flowFileLoader struct {
	matchers   theater.MatcherSugarResolver
	targetPath string
}

type flowLibraryIndex struct {
	byScenarioID map[string][]string
}

func newFlowFileLoader(path string, matchers theater.MatcherSugarResolver) *flowFileLoader {
	return &flowFileLoader{
		matchers:   matchers,
		targetPath: path,
	}
}

func (l *flowFileLoader) Load() (theater.StageSpec, error) {
	result, err := l.LoadDetailed()
	if err != nil {
		return theater.StageSpec{}, err
	}

	return result.Spec, nil
}

func (l *flowFileLoader) LoadDetailed() (LoadResult, error) {
	location, err := authoringyaml.ResolveFlowFileLocation(l.targetPath)
	if err != nil {
		return LoadResult{}, err
	}
	if !location.RepoFound {
		return LoadResult{}, errFlowRepoNotFound(location.Path)
	}
	if !location.InFlowRoot {
		return LoadResult{}, errFlowOutsideRoot(location.Path, location.Layout.FlowRoot)
	}

	flowResult, err := LoadFileDetailed(location.Path, l.matchers)
	if err != nil {
		return LoadResult{}, err
	}

	flowSpec := flowResult.Spec
	neededScenarioIDs := unresolvedFlowScenarioIDs(flowSpec)
	if len(neededScenarioIDs) == 0 {
		return LoadResult{
			Spec:      cloneFlowStage(flowSpec),
			sourceMap: cloneSourceMap(flowResult.sourceMap),
			yamlData:  flowResult.CanonicalYAML(),
		}, nil
	}

	libraryFiles, err := collectFlowLibraryFiles(location.Layout.LibraryRoot)
	if err != nil {
		return LoadResult{}, err
	}

	index, err := buildFlowLibraryIndex(libraryFiles)
	if err != nil {
		return LoadResult{}, err
	}

	return assembleFlowStageDetailed(flowResult, neededScenarioIDs, index, l.matchers, nil)
}

func collectFlowLibraryFiles(libraryRoot string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(libraryRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			if path != libraryRoot && shouldIgnoreLibraryDirectory(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		if !isTHTRFile(path) {
			return nil
		}

		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func assembleFlowStageDetailed(
	flowResult LoadResult,
	neededScenarioIDs map[string]struct{},
	index flowLibraryIndex,
	matchers theater.MatcherSugarResolver,
	libraryOverlay map[string][]byte,
) (LoadResult, error) {
	flowSpec := flowResult.Spec
	assembled := cloneFlowStage(flowSpec)
	selectedFiles, err := selectFlowLibraryFiles(index, neededScenarioIDs)
	if err != nil {
		return LoadResult{}, err
	}

	mergedSourceMap := cloneSourceMap(flowResult.sourceMap)
	for i := range selectedFiles {
		libraryResult, err := loadLibraryDetailed(selectedFiles[i], matchers, libraryOverlay)
		if err != nil {
			return LoadResult{}, err
		}
		librarySpec := libraryResult.Spec
		if len(librarySpec.ScenarioCalls) > 0 {
			return LoadResult{}, fmt.Errorf("library file %s must not declare scenario_calls", selectedFiles[i])
		}

		selectedScenarioIDs := make(map[string]struct{})
		for j := range librarySpec.Scenarios {
			scenario := librarySpec.Scenarios[j]
			if _, needed := neededScenarioIDs[scenario.ID]; !needed {
				continue
			}
			selectedScenarioIDs[scenario.ID] = struct{}{}
			assembled.Scenarios = append(assembled.Scenarios, scenario)
		}

		mergedSourceMap = mergeRebasedScenarioSourceMaps(
			mergedSourceMap,
			libraryResult.sourceMap,
			librarySpec.ID,
			flowSpec.ID,
			selectedScenarioIDs,
		)
	}

	data, err := marshalCanonicalYAML(assembled, mergedSourceMap)
	if err != nil {
		return LoadResult{}, err
	}

	return LoadResult{
		Spec:      assembled,
		sourceMap: mergedSourceMap,
		yamlData:  data,
	}, nil
}

func cloneFlowStage(flowSpec theater.StageSpec) theater.StageSpec {
	return theater.StageSpec{
		ID:            flowSpec.ID,
		Name:          flowSpec.Name,
		HTTP:          flowSpec.HTTP,
		State:         flowSpec.State,
		Scenarios:     append([]theater.ScenarioSpec(nil), flowSpec.Scenarios...),
		ScenarioCalls: append([]theater.ScenarioCallSpec(nil), flowSpec.ScenarioCalls...),
		SourceSpan:    flowSpec.SourceSpan,
	}
}

func buildFlowLibraryIndex(libraryFiles []string) (flowLibraryIndex, error) {
	return buildFlowLibraryIndexWithOverlay(libraryFiles, nil)
}

func buildFlowLibraryIndexWithOverlay(libraryFiles []string, libraryOverlay map[string][]byte) (flowLibraryIndex, error) {
	index := flowLibraryIndex{
		byScenarioID: make(map[string][]string),
	}

	for i := range libraryFiles {
		document, err := loadLibraryDocumentWithOverlay(libraryFiles[i], libraryOverlay)
		if err != nil {
			return flowLibraryIndex{}, err
		}

		for j := range document.Scenarios {
			scenarioID := document.Scenarios[j].ID
			if existing := index.byScenarioID[scenarioID]; len(existing) > 0 {
				if existing[0] == libraryFiles[i] {
					return flowLibraryIndex{}, fmt.Errorf(
						"library scenario %q is declared multiple times in %s",
						scenarioID,
						libraryFiles[i],
					)
				}
				return flowLibraryIndex{}, fmt.Errorf(
					"library scenario %q is declared in multiple files: %s, %s",
					scenarioID,
					existing[0],
					libraryFiles[i],
				)
			}
			index.byScenarioID[scenarioID] = append(index.byScenarioID[scenarioID], libraryFiles[i])
		}
	}

	return index, nil
}

func selectFlowLibraryFiles(index flowLibraryIndex, neededScenarioIDs map[string]struct{}) ([]string, error) {
	selected := make(map[string]struct{})
	files := make([]string, 0)

	for scenarioID := range neededScenarioIDs {
		libraryFiles := index.byScenarioID[scenarioID]
		if len(libraryFiles) == 0 {
			return nil, fmt.Errorf("referenced library scenario %q is not found", scenarioID)
		}

		for i := range libraryFiles {
			if _, ok := selected[libraryFiles[i]]; ok {
				continue
			}

			selected[libraryFiles[i]] = struct{}{}
			files = append(files, libraryFiles[i])
		}
	}

	sort.Strings(files)
	return files, nil
}

func unresolvedFlowScenarioIDs(flowSpec theater.StageSpec) map[string]struct{} {
	local := make(map[string]struct{}, len(flowSpec.Scenarios))
	for i := range flowSpec.Scenarios {
		local[flowSpec.Scenarios[i].ID] = struct{}{}
	}

	needed := make(map[string]struct{})
	for i := range flowSpec.ScenarioCalls {
		scenarioID := flowSpec.ScenarioCalls[i].ScenarioID
		if _, ok := local[scenarioID]; ok {
			continue
		}

		needed[scenarioID] = struct{}{}
	}

	return needed
}

func loadLibraryDocumentWithOverlay(path string, libraryOverlay map[string][]byte) (*syntaxDocument, error) {
	data, err := os.ReadFile(path)
	if overlay, ok := libraryOverlay[canonicalOverlayPath(path)]; ok {
		data = overlay
	} else if err != nil {
		return nil, err
	}

	tokens, err := lex(data)
	if err != nil {
		return nil, err
	}

	document, err := parseTokens(tokens)
	if err != nil {
		return nil, err
	}

	return document, nil
}

func loadLibraryDetailed(path string, matchers theater.MatcherSugarResolver, libraryOverlay map[string][]byte) (LoadResult, error) {
	if overlay, ok := libraryOverlay[canonicalOverlayPath(path)]; ok {
		return ParseDetailed(overlay, path, matchers)
	}

	return LoadFileDetailed(path, matchers)
}

func normalizeSourceOverlay(overlay map[string][]byte) map[string][]byte {
	if len(overlay) == 0 {
		return nil
	}

	normalized := make(map[string][]byte, len(overlay))
	for path, data := range overlay {
		normalized[canonicalOverlayPath(path)] = data
	}
	return normalized
}

func canonicalOverlayPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolvedPath, err := filepath.EvalSymlinks(absPath); err == nil {
		return resolvedPath
	}
	return absPath
}

func isTHTRFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".thtr")
}

func shouldIgnoreLibraryDirectory(name string) bool {
	switch name {
	case "examples", "fixtures", "internal", "testdata":
		return true
	default:
		return false
	}
}

func cloneSourceMap(source *sourceMap) *sourceMap {
	if source == nil {
		return nil
	}

	entries := cloneSourceMapEntries(source.Entries)
	index := make(map[string]int, len(entries))
	for i := range entries {
		index[entries[i].SpecPath] = i
	}

	return &sourceMap{
		Version:    source.Version,
		Entries:    entries,
		bySpecPath: index,
	}
}

func cloneSourceMapEntries(entries []sourceMapEntry) []sourceMapEntry {
	if len(entries) == 0 {
		return nil
	}

	cloned := make([]sourceMapEntry, len(entries))
	for i := range entries {
		cloned[i] = entries[i]
		cloned[i].locator = cloneYAMLPath(entries[i].locator)
		if entries[i].YAML != nil {
			yamlRange := *entries[i].YAML
			cloned[i].YAML = &yamlRange
		}
	}

	return cloned
}

func mergeRebasedScenarioSourceMaps(
	base *sourceMap,
	library *sourceMap,
	libraryStageID string,
	flowStageID string,
	selectedScenarioIDs map[string]struct{},
) *sourceMap {
	if library == nil || len(selectedScenarioIDs) == 0 {
		return base
	}

	merged := cloneSourceMap(base)
	if merged == nil {
		merged = &sourceMap{
			Version:    sourceMapVersion,
			Entries:    nil,
			bySpecPath: make(map[string]int),
		}
	}
	if merged.bySpecPath == nil {
		merged.bySpecPath = make(map[string]int, len(merged.Entries))
		for i := range merged.Entries {
			merged.bySpecPath[merged.Entries[i].SpecPath] = i
		}
	}

	codec := sourcePathCodec{}
	libraryStagePath := codec.Join("stage", libraryStageID)
	flowStagePath := codec.Join("stage", flowStageID)

	for i := range library.Entries {
		entry, ok := rebaseScenarioSourceMapEntry(
			library.Entries[i],
			libraryStagePath,
			flowStagePath,
			selectedScenarioIDs,
		)
		if !ok {
			continue
		}
		if _, exists := merged.bySpecPath[entry.SpecPath]; exists {
			continue
		}

		merged.bySpecPath[entry.SpecPath] = len(merged.Entries)
		merged.Entries = append(merged.Entries, entry)
	}

	return merged
}

func rebaseScenarioSourceMapEntry(
	entry sourceMapEntry,
	libraryStagePath string,
	flowStagePath string,
	selectedScenarioIDs map[string]struct{},
) (sourceMapEntry, bool) {
	codec := sourcePathCodec{}

	for scenarioID := range selectedScenarioIDs {
		libraryScenarioPath := codec.JoinChild(libraryStagePath, "scenario", scenarioID)
		if entry.SpecPath != libraryScenarioPath &&
			!strings.HasPrefix(entry.SpecPath, libraryScenarioPath+"/") {
			continue
		}

		rebased := entry
		rebased.SpecPath = flowStagePath + strings.TrimPrefix(entry.SpecPath, libraryStagePath)
		rebased.NodeID = rebased.SpecPath
		rebased.YAML = nil
		rebased.locator = nil
		return rebased, true
	}

	return sourceMapEntry{}, false
}
