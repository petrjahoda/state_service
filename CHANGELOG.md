# Alarm Service Changelog

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/).

Please note, that this project, while following numbering syntax, it DOES NOT
adhere to [Semantic Versioning](http://semver.org/spec/v2.0.0.html) rules.

## Types of changes

* ```Added``` for new features.
* ```Changed``` for changes in existing functionality.
* ```Deprecated``` for soon-to-be removed features.
* ```Removed``` for now removed features.
* ```Fixed``` for any bug fixes.
* ```Security``` in case of vulnerabilities.

## [2021.1.2.22] - 2021-02-22

### Changed
- one database connection per workplace loop

## [2021.1.2.21] - 2021-02-21

### Changed
- updated to latest go
- updated to latest libraries

### Fixed
- when updating workplace with values, updating just what is needed

## [2020.4.3.14] - 2020-12-14

### Changed
- updated to latest go
- updated to latest libraries

## [2020.4.2.17] - 2020-11-17

### Changed
- updated all go libraries 
- updated Dockerfile
- updated create.sh

## [2020.4.1.26] - 2020-10-26

### Fixed
- fixed leaking goroutine bug when opening sql connections, the right way is this way

## [2020.3.3.29] - 2020-09-29

### Changed
- clearing slice of intermediate data to lower the memory consumption

## [2020.3.3.25] - 2020-09-25

### Changed
- updated readme.md

## [2020.3.2.22] - 2020-08-29

### Changed
- functions naming changed to idiomatic go (ThisFunction -> thisFunction)

## [2020.3.2.22] - 2020-08-22

### Added
- automatic go get -u all when creating docker image

## [2020.3.2.13] - 2020-08-13

### Changed
- updated to latest libraries
- updated to go 1.15 -> final program size reduced to 90%

## [2020.3.2.5] - 2020-08-05

### Added
- MIT license

### Changed
- code moved to more files
- logging updated
- name of functions updated

## [2020.3.2.4] - 2020-08-04

### Changed
- updated to latest libraries

### Added
- added coloring to logging when changing states

## [2020.3.1.30] - 2020-07-30

### Fixed
- proper closing database connections with sqlDB, err := db.DB() and defer sqlDB.Close()

### Changed
- added tzdata to docker image

## [2020.3.1.26] - 2020-07-26

### Changed
- changed to gorm v2
- postgres only

### Removed
- logging to file
- all about config file

## [2020.2.2.18] - 2020-05-18

### Added
- init for actual service directory
- db.logmode(false)

## [2020.2.2.8] - 2020-05-08

### Changed
- when downloading data from database for processing, downloading smallest amount of data possible

## [2020.1.3.31] - 2020-03-31

### Added
- updated create.sh for uploading proper docker version automatically

## [2020.1.2.29] - 2020-02-29

### Changed
- name of database changed to zapsi3
- proper testing for mariadb, postgres and mssql
- added logging for all important methods and functions
- code refactoring for better readability
