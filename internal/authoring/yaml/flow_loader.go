package yaml

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alex-poliushkin/theater"
)

const (
	flowLibraryRootName = "lib"
	flowStageRootName   = "flows"
	flowRepoRootName    = "theater"
)

var ignoredLibraryDirectoryNames = map[string]struct{}{
	"examples": {},
	"fixtures": {},
	"internal": {},
	"testdata": {},
}

func LoadFlowFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	loader := newFlowFileLoader(path, matchers)
	return loader.Load()
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
	location, err := ResolveFlowFileLocation(l.targetPath)
	if err != nil {
		return theater.StageSpec{}, err
	}
	if !location.RepoFound {
		return theater.StageSpec{}, fmt.Errorf("repo-local theater roots not found for flow file %s", location.Path)
	}
	if !location.InFlowRoot {
		return theater.StageSpec{}, fmt.Errorf("flow file must be located under %s", location.Layout.FlowRoot)
	}

	flowSpec, err := loadFlowStage(location.Path, l.matchers)
	if err != nil {
		return theater.StageSpec{}, err
	}

	neededScenarioIDs := unresolvedFlowScenarioIDs(flowSpec)
	if len(neededScenarioIDs) == 0 {
		return cloneFlowStage(flowSpec), nil
	}

	libraryFiles, err := collectLibraryFiles(location.Layout.LibraryRoot)
	if err != nil {
		return theater.StageSpec{}, err
	}

	index, err := buildFlowLibraryIndex(libraryFiles)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return assembleFlowStage(flowSpec, neededScenarioIDs, index, l.matchers)
}

func collectLibraryFiles(libraryRoot string) ([]string, error) {
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

		if !isYAMLFile(path) {
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

func assembleFlowStage(
	flowSpec theater.StageSpec,
	neededScenarioIDs map[string]struct{},
	index flowLibraryIndex,
	matchers theater.MatcherSugarResolver,
) (theater.StageSpec, error) {
	assembled := cloneFlowStage(flowSpec)
	selectedFiles, err := selectFlowLibraryFiles(index, neededScenarioIDs)
	if err != nil {
		return theater.StageSpec{}, err
	}

	for _, libraryFile := range selectedFiles {
		librarySpec, err := loadLibraryStage(libraryFile, matchers)
		if err != nil {
			return theater.StageSpec{}, err
		}

		if len(librarySpec.ScenarioCalls) > 0 {
			return theater.StageSpec{}, fmt.Errorf("library file %s must not declare scenario_calls", libraryFile)
		}

		for i := range librarySpec.Scenarios {
			scenario := librarySpec.Scenarios[i]
			if _, needed := neededScenarioIDs[scenario.ID]; !needed {
				continue
			}

			assembled.Scenarios = append(assembled.Scenarios, scenario)
		}
	}

	return assembled, nil
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

func loadFlowStage(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return LoadFile(path, matchers)
}

func loadLibraryStage(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return LoadFile(path, matchers)
}

func buildFlowLibraryIndex(libraryFiles []string) (flowLibraryIndex, error) {
	index := flowLibraryIndex{
		byScenarioID: make(map[string][]string),
	}

	for _, libraryFile := range libraryFiles {
		raw, err := loadRawLibraryStage(libraryFile)
		if err != nil {
			return flowLibraryIndex{}, err
		}

		for i := range raw.Scenarios {
			scenarioID := raw.Scenarios[i].ID
			if existing := index.byScenarioID[scenarioID]; len(existing) > 0 {
				if existing[0] == libraryFile {
					return flowLibraryIndex{}, fmt.Errorf(
						"library scenario %q is declared multiple times in %s",
						scenarioID,
						libraryFile,
					)
				}
				return flowLibraryIndex{}, fmt.Errorf(
					"library scenario %q is declared in multiple files: %s, %s",
					scenarioID,
					existing[0],
					libraryFile,
				)
			}
			index.byScenarioID[scenarioID] = append(index.byScenarioID[scenarioID], libraryFile)
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

		for _, libraryFile := range libraryFiles {
			if _, ok := selected[libraryFile]; ok {
				continue
			}

			selected[libraryFile] = struct{}{}
			files = append(files, libraryFile)
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

func loadRawLibraryStage(path string) (rawStageSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rawStageSpec{}, err
	}

	return decodeRawStage(bytes.NewReader(data))
}

func isYAMLFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func shouldIgnoreLibraryDirectory(name string) bool {
	_, ignored := ignoredLibraryDirectoryNames[name]
	return ignored
}
