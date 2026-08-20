---
layout: default
title: Kubernetes Cloud Controller Manager for Linode
nav_order: 1
---

{%- capture readme -%}
{%- include_relative README.md -%}
{%- endcapture -%}

{{ readme
  | replace: ".md)", ".html)"
  | replace: ".md#", ".html#"
  | replace: "](.github/CONTRIBUTING.html)", "](<https://github.com/linode/linode-cloud-controller-manager/blob/main/.github/CONTRIBUTING.md>)"
}}
