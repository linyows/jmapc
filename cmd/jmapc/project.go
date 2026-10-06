package main

import "flag"

// projectFlags are the flags the commands reading a project's settings take:
// the settings file, the directory holding the requests, and the schemas of
// vendor extensions. The directory replaces the one the settings file names,
// and the schemas are read as well as the ones it names.
type projectFlags struct {
	config   *string
	requests *string
	schemas  stringList
}

// addProjectFlags declares the flags on fs. A command that reads no requests,
// as schema does not, is given no -requests.
func addProjectFlags(fs *flag.FlagSet, requests bool) *projectFlags {
	f := &projectFlags{config: fs.String("config", "", "settings file to read")}
	if requests {
		f.requests = fs.String("requests", "", "directory holding the request files")
	}
	fs.Var(&f.schemas, "schema", "schema file describing a vendor extension; repeatable")
	return f
}

// settings reads the settings file and lays the flags over it: -requests in
// place of what it says, and -schema after the schemas it lists. The defaults
// are left to the caller, which may have flags of its own to lay over first.
func (f *projectFlags) settings() (*Config, error) {
	cfg, err := loadConfig(*f.config)
	if err != nil {
		return nil, err
	}
	if f.requests != nil && *f.requests != "" {
		cfg.Requests = *f.requests
	}
	if len(f.schemas) > 0 {
		cfg.Schemas = append(cfg.Schemas, f.schemas...)
	}
	return cfg, nil
}
