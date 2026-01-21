package ccm

import (
	"fmt"
	"text/template"

	"github.com/outscale/goutils/k8s/sdk"
	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
)

type Options struct {
	LBTags             map[string]string
	NodeLabels         map[string]string
	nodeLabelTemplates map[string]*template.Template

	sdkOpts sdk.Options
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.Var(cliflag.NewMapStringString(&o.LBTags), "extra-loadbalancer-tags", "Extra tags to add to load-balancers. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
	fs.Var(cliflag.NewMapStringString(&o.NodeLabels), "extra-node-labels", "Extra labels to add to nodes. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
	o.sdkOpts.AddFlags(fs)
}

func (o *Options) Compile() error {
	o.nodeLabelTemplates = make(map[string]*template.Template, len(o.NodeLabels))
	for k, v := range o.NodeLabels {
		var err error
		o.nodeLabelTemplates[k], err = template.New(k).Parse(v)
		if err != nil {
			return fmt.Errorf("invalid label value %q: %w", v, err)
		}
	}
	return nil
}
