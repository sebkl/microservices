# microservices

This microservices repository contains a set of minimal web-services which are frequently used by myself for a wide range of individual applications, tools and services. They have been extracted for convinience purposes and may act as example services for my golang [GOTOJS](http://godoc.org/github.com/sebkl/gotojs)  package.


## Usage

Currently there are two types of microservices:

* Plain **gotojs** services
* Services that could be used as **appengine** modules

### gotojs

```TBD```

### appengine
The appengine service do also contain some basic html5 web frontend which also shall act as sample applications.

```TBD```

####Example 'app.yaml' configuration:
```yaml
application: <application_name>
version: dev
runtime: go
api_version: go1
module: shareme
instance_class: F1
automatic_scaling:
        max_idle_instances: 1
        max_concurrent_requests: 10
        

handlers:
- url: /.*
  script: _go_app
```
