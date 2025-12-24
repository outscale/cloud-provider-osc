package ccm

import (
	"github.com/outscale/goutils/k8s/sdk"
	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
)

type Options struct {
	ExtraTags map[string]string

	sdkOpts sdk.Options
}

func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.Var(cliflag.NewMapStringString(&o.ExtraTags), "extra-loadbalancer-tags", "Extra tags to add to created load-balancers. It is a comma separated list of key value pairs like '<key1>=<value1>,<key2>=<value2>'")
	o.sdkOpts.AddFlags(fs)
}
