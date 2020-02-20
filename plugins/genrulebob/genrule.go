// +build soong

/*
 * Copyright 2020 Arm Limited.
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
package genrulebob

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"android/soong/android"
	"android/soong/cc"
	"android/soong/genrule"

	"github.com/ARM-software/bob-build/internal/utils"

	"github.com/google/blueprint"
)

type MultiOutProps struct {
	Match         string
	Replace       []string
	Implicit_srcs []string
	Implicit_outs []string
}

type GenruleProps struct {
	Srcs                    []string
	Out                     []string
	Implicit_srcs           []string
	Implicit_outs           []string
	Export_gen_include_dirs []string
	Cmd                     string
	Host_bin                string
	Tool                    string
	Depfile                 bool
	Module_deps             []string
	Module_srcs             []string
	Encapsulates            []string
	Cflags                  []string
	Conlyflags              []string
	Cxxflags                []string
	Asflags                 []string
	Ldflags                 []string
	Ldlibs                  []string
	Rsp_content             *string

	Multi_out_srcs  []string
	Multi_out_props MultiOutProps
}

type genruleInterface interface {
	genrule.SourceFileGenerator

	outputs() android.WritablePaths
	outputPath() android.Path
}

type genrulebob struct {
	android.ModuleBase
	Properties GenruleProps

	genDir               android.Path
	exportGenIncludeDirs android.Paths
	inouts               []soongInout
}

type generatedSourceTagType struct {
	blueprint.BaseDependencyTag
}

type generatedDepTagType struct {
	blueprint.BaseDependencyTag
}

type encapsulatesTagType struct {
	blueprint.BaseDependencyTag
}

type hostToolBinTagType struct {
	blueprint.BaseDependencyTag
}

var (
	pctx = android.NewPackageContext("plugins/genrulebob")

	generatedSourceTag generatedSourceTagType
	generatedDepTag    generatedDepTagType
	encapsulatesTag    encapsulatesTagType
	hostToolBinTag     hostToolBinTagType
)

// interfaces implemented
var _ android.Module = (*genrulebob)(nil)
var _ genrule.SourceFileGenerator = (*genrulebob)(nil)
var _ android.AndroidMkEntriesProvider = (*genrulebob)(nil)

func GenruleFactory() android.Module {
	m := &genrulebob{}
	// register all structs that contain module properties (parsable from .bp file)
	// note: we register our custom properties first, to take precedence before common ones
	m.AddProperties(&m.Properties)
	android.InitAndroidModule(m)
	return m
}

func init() {
	android.RegisterModuleType("genrule_bob", GenruleFactory)
}

func (m *genrulebob) outputPath() android.Path {
	return m.genDir
}

func (m *genrulebob) outputs() (ret android.WritablePaths) {
	for _, io := range m.inouts {
		ret = append(ret, io.out...)
		ret = append(ret, io.implicitOuts...)
	}
	return
}

func (m *genrulebob) filterOutputs(predicate func(string) bool) (ret android.Paths) {
	for _, p := range m.outputs() {
		if predicate(p.String()) {
			ret = append(ret, p)
		}
	}
	return
}

func pathsForModuleGen(ctx android.ModuleContext, paths []string) (ret android.WritablePaths) {
	for _, path := range paths {
		ret = append(ret, android.PathForModuleGen(ctx, path))
	}
	return
}

// GeneratedSourceFiles, GeneratedHeaderDirs and GeneratedDeps implement the
// genrule.SourceFileGenerator interface, which allows these modules to be used
// to generate inputs for cc_library and cc_binary modules.
func (m *genrulebob) GeneratedSourceFiles() android.Paths {
	return m.filterOutputs(utils.IsCompilableSource)
}

func (m *genrulebob) GeneratedHeaderDirs() android.Paths {
	return m.exportGenIncludeDirs
}

func (m *genrulebob) GeneratedDeps() (srcs android.Paths) {
	return m.filterOutputs(utils.IsNotCompilableSource)
}

func (m *genrulebob) DepsMutator(mctx android.BottomUpMutatorContext) {
	if m.Properties.Host_bin != "" {
		mctx.AddFarVariationDependencies(mctx.Config().BuildOSTarget.Variations(),
			hostToolBinTag, m.Properties.Host_bin)
	}

	// `module_deps` and `module_srcs` can refer not only to source
	// generation modules, but to binaries and libraries. In this case we
	// need to handle multilib builds, where a 'target' library could be
	// split into 32 and 64-bit variants. Use `AddFarVariationDependencies`
	// here, because this will automatically choose the first available
	// variant, rather than the other dependency-adding functions, which
	// will error when multiple variants are present.
	mctx.AddFarVariationDependencies(nil, generatedDepTag, m.Properties.Module_deps...)
	mctx.AddFarVariationDependencies(nil, generatedSourceTag, m.Properties.Module_srcs...)
	// We can only encapsulate other generated/transformed source modules,
	// so use the normal `AddDependency` function for these.
	mctx.AddDependency(mctx.Module(), encapsulatesTag, m.Properties.Encapsulates...)
}

func (m *genrulebob) getHostBin(ctx android.ModuleContext) android.OptionalPath {
	if m.Properties.Host_bin == "" {
		return android.OptionalPath{}
	}
	hostBinModule := ctx.GetDirectDepWithTag(m.Properties.Host_bin, hostToolBinTag)
	htp, ok := hostBinModule.(genrule.HostToolProvider)
	if !ok {
		panic(fmt.Errorf("%s is not a host tool", m.Properties.Host_bin))
	}
	return htp.HostToolPath()
}

func (m *genrulebob) getArgs(ctx android.ModuleContext) (args map[string]string, dependents []android.Path) {
	dependents = android.PathsForModuleSrc(ctx, m.Properties.Implicit_srcs)
	args = map[string]string{
		"gen_dir":    android.PathForModuleGen(ctx).String(),
		"asflags":    utils.Join(m.Properties.Asflags),
		"cflags":     utils.Join(m.Properties.Cflags),
		"conlyflags": utils.Join(m.Properties.Conlyflags),
		"cxxflags":   utils.Join(m.Properties.Cxxflags),
		"ldflags":    utils.Join(m.Properties.Ldflags),
		"ldlibs":     utils.Join(m.Properties.Ldlibs),

		// flag_defaults is primarily used to invoke sub-makes of
		// different libraries. This shouldn't be needed on Android.
		// This means the following can't be expanded:
		"ar":     "",
		"as":     "",
		"cc":     "",
		"cxx":    "",
		"linker": "",
	}

	// Add arguments providing information about other modules the current
	// one depends on, accessible via ${module}_out and ${module}_dir.
	ctx.VisitDirectDepsWithTag(generatedDepTag, func(dep android.Module) {
		if gdep, ok := dep.(genruleInterface); ok {
			outs := gdep.outputs()
			dependents = append(dependents, outs.Paths()...)

			args[dep.Name()+"_dir"] = gdep.outputPath().String()
			args[dep.Name()+"_out"] = strings.Join(outs.Strings(), " ")
		} else if ccmod, ok := dep.(cc.LinkableInterface); ok {
			out := ccmod.OutputFile()
			dependents = append(dependents, out.Path())
			// We only expect to use the output from static/shared libraries
			// and binaries, so `_dir' is not supported on these.
			args[dep.Name()+"_out"] = out.String()
		}
	})

	return
}

type soongInout struct {
	in           android.Paths
	out          android.WritablePaths
	depfile      android.WritablePath
	implicitSrcs android.Paths
	implicitOuts android.WritablePaths
	rspfile      android.WritablePath
}

func (m *genrulebob) buildInouts(ctx android.ModuleContext, args map[string]string) {
	ruleparams := blueprint.RuleParams{
		Command: m.Properties.Cmd,
		Restat:  true,
	}

	if m.Properties.Depfile {
		args["depfile"] = ""
	}
	args["headers_generated"] = ""
	args["srcs_generated"] = ""

	if m.Properties.Rsp_content != nil {
		args["rspfile"] = ""
		ruleparams.Rspfile = "${rspfile}"
		ruleparams.RspfileContent = *m.Properties.Rsp_content
	}

	rule := ctx.Rule(pctx, "bob_gen_"+ctx.ModuleName(), ruleparams, utils.SortedKeys(args)...)

	for _, sio := range m.inouts {
		// `args` is slightly different for each inout, but blueprint's
		// parseBuildParams() function makes a deep copy of the map, so
		// we're OK to re-use it for each target.
		if m.Properties.Depfile {
			args["depfile"] = sio.depfile.String()
		}
		if m.Properties.Rsp_content != nil {
			args["rspfile"] = sio.rspfile.String()
		}
		args["headers_generated"] = strings.Join(utils.Filter(utils.IsHeader, sio.out.Strings()), " ")
		args["srcs_generated"] = strings.Join(utils.Filter(utils.IsNotHeader, sio.out.Strings()), " ")

		ctx.Build(pctx,
			android.BuildParams{
				Rule:            rule,
				Description:     "gen " + ctx.ModuleName(),
				Inputs:          sio.in,
				Implicits:       sio.implicitSrcs,
				Outputs:         sio.out,
				ImplicitOutputs: sio.implicitOuts,
				Args:            args,
				Depfile:         sio.depfile,
			})
	}
}

func (m *genrulebob) calcExportGenIncludeDirs(mctx android.ModuleContext) android.Paths {
	var allIncludeDirs android.Paths

	// Add our own include dirs
	for _, dir := range m.Properties.Export_gen_include_dirs {
		allIncludeDirs = append(allIncludeDirs, android.PathForModuleGen(mctx, dir))
	}

	// Add include dirs of our all dependencies
	mctx.WalkDeps(func(child android.Module, parent android.Module) bool {
		if mctx.OtherModuleDependencyTag(child) != encapsulatesTag {
			return false
		}
		if cmod, ok := child.(genruleInterface); ok {
			for _, dir := range cmod.GeneratedHeaderDirs() {
				allIncludeDirs = append(allIncludeDirs, dir)
			}
		}
		return true
	})

	// Make unique items as for recursive passes it may contain redundant ones
	return android.FirstUniquePaths(allIncludeDirs)
}

func getDepfileName(s string) string {
	return s + ".d"
}

// Remove the relative part from android.Path
func nonRelPathString(path android.Path) string {
	return strings.TrimSuffix(path.String(), path.Rel())
}

func pathsForImplicitSrcs(ctx android.ModuleContext, source android.Path, props []string) (paths android.Paths) {
	if _, ok := source.(android.ModuleGenPath); ok {
		// Remove the build directory from the path since android.PathForOutput is going to add it
		nonRelString := android.Rel(ctx, ctx.Config().BuildDir(), nonRelPathString(source))
		// Convert to android.OutputPath
		nonRel := android.PathForOutput(ctx, nonRelString)
		for _, prop := range props {
			paths = append(paths, nonRel.Join(ctx, prop))
		}
	} else {
		nonRel := nonRelPathString(source)
		for _, prop := range props {
			paths = append(paths, android.PathForSource(ctx, filepath.Join(nonRel, prop)))
		}
	}
	return
}

func (m *genrulebob) inoutForSrc(ctx android.ModuleContext, re *regexp.Regexp, source android.Path) (sio soongInout) {
	replaceSource := func(props []string) (newProps []string) {
		for _, prop := range props {
			newProps = append(newProps, re.ReplaceAllString(source.Rel(), prop))
		}
		return
	}
	mop := m.Properties.Multi_out_props

	sio.in = android.Paths{source}
	sio.out = pathsForModuleGen(ctx, replaceSource(mop.Replace))
	sio.implicitSrcs = pathsForImplicitSrcs(ctx, source, replaceSource(mop.Implicit_srcs))
	sio.implicitOuts = pathsForModuleGen(ctx, replaceSource(mop.Implicit_outs))

	if m.Properties.Depfile {
		sio.depfile = android.PathForModuleGen(ctx, getDepfileName(filepath.Base(source.Rel())))
	}

	if m.Properties.Rsp_content != nil {
		sio.rspfile = android.PathForModuleGen(ctx, filepath.Dir(source.Rel()),
			"."+filepath.Base(source.Rel())+".rsp")
	}

	return
}

func (m *genrulebob) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	args, implicits := m.getArgs(ctx)

	m.genDir = android.PathForModuleGen(ctx)
	m.exportGenIncludeDirs = m.calcExportGenIncludeDirs(ctx)

	if hostBin := m.getHostBin(ctx); hostBin.Valid() {
		args["host_bin"] = hostBin.String()
		implicits = append(implicits, hostBin.Path())
	}

	if m.Properties.Tool != "" {
		tool := android.PathForModuleSrc(ctx, m.Properties.Tool)
		args["tool"] = tool.String()
		implicits = append(implicits, tool)
	}

	if len(m.Properties.Out) > 0 {
		sio := soongInout{
			in:           android.PathsForModuleSrc(ctx, m.Properties.Srcs),
			implicitSrcs: implicits,
			out:          pathsForModuleGen(ctx, m.Properties.Out),
			implicitOuts: pathsForModuleGen(ctx, m.Properties.Implicit_outs),
		}
		if m.Properties.Depfile {
			sio.depfile = android.PathForModuleGen(ctx, getDepfileName(m.Name()))
		}
		if m.Properties.Rsp_content != nil {
			sio.rspfile = android.PathForModuleGen(ctx, "."+m.Name()+".rsp")
		}

		m.inouts = append(m.inouts, sio)
	}

	re := regexp.MustCompile(m.Properties.Multi_out_props.Match)
	for _, tsrc := range m.Properties.Multi_out_srcs {
		m.inouts = append(m.inouts, m.inoutForSrc(ctx, re, android.PathForModuleSrc(ctx, tsrc)))
	}

	m.buildInouts(ctx, args)
}

func (m *genrulebob) AndroidMkEntries() []android.AndroidMkEntries {
	entries := []android.AndroidMkEntries{}
	for _, inout := range m.inouts {
		for _, outfile := range inout.out {

			entries = append(entries, android.AndroidMkEntries{
				Class:      "DATA",
				OutputFile: android.OptionalPathForPath(outfile),
				// if module has more than one output, keep LOCAL_MODULE unique
				SubName: "__" + strings.Replace(outfile.Rel(), "/", "__", -1),
				Include: "$(BUILD_PREBUILT)",
				ExtraEntries: []android.AndroidMkExtraEntriesFunc{
					func(entries *android.AndroidMkEntries) {
						entries.SetBool("LOCAL_UNINSTALLABLE_MODULE", true)
					},
				},
			})

		}
	}
	return entries
}