/*
 * Copyright 2018-2020 Arm Limited.
 * SPDX-License-Identifier: Apache-2.0
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package core

import (
	"github.com/google/blueprint"
)

type generateStaticLibrary struct {
	generateLibrary
}

// Verify that the following interfaces are implemented
var _ generateLibraryInterface = (*generateStaticLibrary)(nil)
var _ singleOutputModule = (*generateStaticLibrary)(nil)
var _ blueprint.Module = (*generateStaticLibrary)(nil)

func (m *generateStaticLibrary) generateInouts(ctx blueprint.ModuleContext, g generatorBackend) []inout {
	return generateLibraryInouts(m, ctx, g, m.Properties.Headers)
}

//// Support generateLibraryInterface

func (m *generateStaticLibrary) libExtension() string {
	return ".a"
}

//// Support blueprint.Module

func (m *generateStaticLibrary) GenerateBuildActions(ctx blueprint.ModuleContext) {
	if isEnabled(m) {
		g := getBackend(ctx)
		g.genStaticActions(m, ctx)
	}
}

//// Support singleOutputModule

func (m *generateStaticLibrary) outputFileName() string {
	return m.altName() + m.libExtension()
}

//// Factory functions

func genStaticLibFactory(config *bobConfig) (blueprint.Module, []interface{}) {
	module := &generateStaticLibrary{}
	module.generateCommon.init(&config.Properties, GenerateProps{},
		GenerateLibraryProps{})

	return module, []interface{}{
		&module.SimpleName.Properties,
		&module.generateCommon.Properties,
		&module.Properties,
	}
}
