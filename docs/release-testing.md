With every rancher release, a new operator version may or may not be released. _This document provides a reference to what tests should be run for both the cases._

For release testing, we test against development Rancher builds; for e.g. to test 2.10, we use `latest/devel/2.10` rancher version (`v2.10-head` image tag). These builds are similar to RC builds, as both utilize the `dev-v2.x` branch of the Rancher chart repository, where application charts for operators are prepared for upcoming release.

Before validation, always sync with developers to ensure the desired versions have been merged into the charts and Rancher projects.

_`P0` suite (`p0_provisioning/p0_import`) must be run to test every new rancher release_, preferably from Rancher Dashboard (manually using UI). 

If the operator contains any update, the [entire test suite](https://app.qase.io/project/HP) must be tested except `K8sChartSupportUpgrade*`; unless the operator also adds support for a new K8s version.

GH action `tests_to_run` would be:
- `p0_provisioning/p0_import` - tests provisioning and importing clusters, k8s upgrade of the control plane and nodes, and CRUD operations of the nodegroup/nodepool
- `support_matrix_provisioning/support_matrix_import` - test all the supported K8s versions
- `p1_provisioning/p1_import` - test P1 Level test cases
- `sync_provisioning/sync_import` - is also P1 Level test case, but more specifically tests sync between Cloud Console and Rancher.
- `k8s_chart_support_provisioning/k8s_chart_support_import` - tests operator chart installation/uninstallation/downgrade/upgrade
- `k8s_chart_support_upgrade_provisioning/k8s_chart_support_upgrade_import` using [k8s-chart-support-matrix](https://github.com/rancher/hosted-providers-e2e/actions/workflows/k8s-chart-upgrade-matrix.yaml) if operator adds support for a new k8s version - similar to `K8sChartSupport*` but also tests rancher and k8s version upgrade.

[Release-matrix](https://github.com/rancher/hosted-providers-e2e/actions/workflows/release-matrix.yaml) GH action can be triggered to test all the providers at once.

If the operator does not contain any update, running `P0*` from Rancher Dashboard should suffice for prime-only rancher release (for e.g 2.8, 2.9; at the time of writing this doc), along with some ad-hoc UI testing for a non-GA rancher minor version (for e.g. v2.11).
