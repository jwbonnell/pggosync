#!/bin/bash

pggosync group -g base -g country_var_1:33
pggosync group -g base -g country_var_1:33 --schema
pggosync sync --table country:"WHERE country_id = 1"