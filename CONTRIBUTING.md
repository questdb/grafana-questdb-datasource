# Contributing to QuestDB Datasource

Thank you for your interest in contributing to this repository. We are glad you want to help us to improve the project and join our community. 
Feel free to [browse the open issues](https://github.com/questdb/questdb-grafana-datasource/issues).  

## Development setup

### Getting started

Clone this repository into your local environment. The frontend code lives in the `src` folder, alongside the [plugin.json file](https://grafana.com/docs/grafana/latest/developers/plugins/metadata/). 
The backend Go code is in the `pkg` folder. To build this plugin refer to [plugin tools(https://grafana.com/developers/plugin-tools).

### Running the development version

Before you can set up the plugin, you need to set up your environment by installing:
- [golang 1.21+](https://go.dev/doc/install)
- [nodejs 18+](https://nodejs.org/en/download)
- [mage](https://github.com/magefile/mage)
- [yarn](https://classic.yarnpkg.com/en/docs/install) 

#### Compiling the backend

You can use [mage](https://github.com/magefile/mage) to compile and test the Go backend.

```sh
mage test # run all Go test cases
mage build:backend && mage reloadPlugin # builds and reloads the plugin in Grafana
```

If host operating system is different from one used in grafana docker image use the following instead: 

```sh
mage -v buildAll
```

#### Compiling the frontend

You can build and test the frontend by using `yarn`:

```sh
yarn test # run all test cases
yarn dev # builds and puts the output at ./dist
```

You can also have `yarn` watch for changes and automatically recompile them:

```sh
yarn watch
```

### Starting docker containers 

You can start grafana and questdb containers with:

```sh
docker-compose up
```
