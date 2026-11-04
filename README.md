# DayTrader 3: WebSphere → Spring Boot modernization "before" estate

This fork is prepared as the starting point for a WebSphere (Liberty / synthetic tWAS on AIX) → Spring Boot on AWS ECS Fargate migration demo.

```bash
make up && make build && make run && make parity TARGET=http://localhost:9083/daytrader
```

- [AGENTS.md](AGENTS.md): commands and rules for agents
- [docs/BASELINE.md](docs/BASELINE.md): how the legacy app builds and runs today
- [docs/WEBSPHERE_INVENTORY.md](docs/WEBSPHERE_INVENTORY.md): generated WebSphere dependency inventory
- [docs/TARGET_STATE.md](docs/TARGET_STATE.md): target architecture
- [docs/LAB.md](docs/LAB.md): live-demo prompts
- [parity/](parity/NORMALIZATION.md): behavioural parity harness
- [legacy-ops/](legacy-ops/README.md): **synthetic** traditional WebSphere on AIX operations artefacts

The original README follows.

---

# sample.daytrader3 [![Build Status](https://travis-ci.org/WASdev/sample.daytrader3.svg?branch=master)](https://travis-ci.org/WASdev/sample.daytrader3)

# Java EE6: DayTrader3 Sample

Java EE6 DayTrader3 Sample


This sample contains the DayTrader 3 benchmark, which is an application built around the paradigm of an online stock trading system. The application allows users to login, view their portfolio, lookup stock quotes, and buy or sell stock shares. With the aid of a Web-based load driver such as Apache JMeter, the real-world workload provided by DayTrader can be used to measure and compare the performance of Java Platform, Enterprise Edition (Java EE) application servers offered by a variety of vendors. In addition to the full workload, the application also contains a set of primitives used for functional and performance testing of various Java EE components and common design patterns.

DayTrader is an end-to-end benchmark and performance sample application. It provides a real world Java EE workload. 

## Getting Started

Browse the code to see what it does, or build and run it yourself:
* [Building and running on the command line using Maven and Gradle](/docs/Using-cmd-line.md)
* [Building and running using Eclipse and WebSphere Development Tools (WDT)](/docs/Using-WDT.md)
* [Downloading WAS Liberty](/docs/Downloading-WAS-Liberty.md)

Once the server has been started, go to [http://localhost:9083/daytrader](http://localhost:9083/daytrader) to interact with the sample.

## Notice

© Copyright IBM Corporation 2015.

## License

```text
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
````
