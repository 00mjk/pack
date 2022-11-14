package fakes

import (
	"github.com/buildpacks/pack/internal/builder"
	"github.com/buildpacks/pack/pkg/dist"
)

type FakeLabelManager struct {
	ReturnForMetadata        builder.Metadata
	ReturnForStackID         string
	ReturnForMixins          []string
	ReturnForOrder           dist.Order
	ReturnForBuildpackLayers dist.ModuleLayers

	ErrorForMetadata        error
	ErrorForStackID         error
	ErrorForMixins          error
	ErrorForOrder           error
	ErrorForBuildpackLayers error
}

func (m *FakeLabelManager) Metadata() (builder.Metadata, error) {
	return m.ReturnForMetadata, m.ErrorForMetadata
}

func (m *FakeLabelManager) StackID() (string, error) {
	return m.ReturnForStackID, m.ErrorForStackID
}

func (m *FakeLabelManager) Mixins() ([]string, error) {
	return m.ReturnForMixins, m.ErrorForMixins
}

func (m *FakeLabelManager) Order() (dist.Order, error) {
	return m.ReturnForOrder, m.ErrorForOrder
}

func (m *FakeLabelManager) BuildpackLayers() (dist.ModuleLayers, error) {
	return m.ReturnForBuildpackLayers, m.ErrorForBuildpackLayers
}
