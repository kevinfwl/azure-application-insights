/*
 * Copyright 2018-2020 the original author or authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package trace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpacks/libcnb"
	"github.com/magiconair/properties"
	"github.com/paketo-buildpacks/libpak"
	"github.com/paketo-buildpacks/libpak/bard"
	"github.com/paketo-buildpacks/libpak/bindings"
	"github.com/paketo-buildpacks/libpak/sherpa"
)

type JavaAgent struct {
	BuildpackPath    string
	Context          libcnb.BuildContext
	LayerContributor libpak.DependencyLayerContributor
	Logger           bard.Logger
}

func NewJavaAgent(buildpackPath string, dependency libpak.BuildpackDependency, cache libpak.DependencyCache,
	plan *libcnb.BuildpackPlan, context libcnb.BuildContext) JavaAgent {

	return JavaAgent{
		Context:          context,
		BuildpackPath:    buildpackPath,
		LayerContributor: libpak.NewDependencyLayerContributor(dependency, cache, plan),
	}
}

func (j JavaAgent) Contribute(layer libcnb.Layer) (libcnb.Layer, error) {
	j.LayerContributor.Logger = j.Logger

	return j.LayerContributor.Contribute(layer, func(artifact *os.File) (libcnb.Layer, error) {
		j.Logger.Bodyf("Copying to %s", layer.Path)

		file := filepath.Join(layer.Path, filepath.Base(artifact.Name()))
		if err := sherpa.CopyFile(artifact, file); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to copy %s to %s\n%w", artifact.Name(), file, err)
		}

		toolOpsDelim := " "
		toolOpsFormat := "-javaagent:" + file

		//enables settings the configuration with the "dd-agent-config.properties" file
		binding, ok, err := bindings.ResolveOne(j.Context.Platform.Bindings, bindings.OfType("DatadogTrace"))

		//we need to test if this works properly
		if ok {
			args, err := handleAgentProperties(binding)
			if err != nil {
				return libcnb.Layer{}, fmt.Errorf("unable to process maven settings from binding\n%w", err)
			}
			toolOpsFormat += args
		}

		layer.LaunchEnvironment.Appendf("JAVA_TOOL_OPTIONS", toolOpsDelim, toolOpsFormat)

		//TODO: this needs to be removed and replaced with other
		file = filepath.Join(j.BuildpackPath, "resources", "dd-trace-agent.xml")

		in, err := os.Open(file)
		if err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to open %s\n%w", file, err)
		}
		defer in.Close()

		file = filepath.Join(layer.Path, "dd-trace-agent.xml")
		if err := sherpa.CopyFile(in, file); err != nil {
			return libcnb.Layer{}, fmt.Errorf("unable to copy %s to %s\n%w", in.Name(), file, err)
		}

		return layer, nil
	}, libpak.LaunchLayer)
}

func (j JavaAgent) Name() string {
	return j.LayerContributor.LayerName()
}

//enables running a java agent based on a file
func handleAgentProperties(binding libcnb.Binding) (string, error) {
	path, ok := binding.SecretFilePath("dd-agent-config.properties")
	launchProperties := ""
	if !ok {
		return launchProperties, nil
	}
	p := properties.MustLoadFile(path, properties.UTF8)
	keys := p.Keys()
	propertyMap := p.Map()

	for i, s := range keys {
		_ = i
		launchProperties += " -D" + s + "=" + propertyMap[s]
	}

	return launchProperties, nil
}
