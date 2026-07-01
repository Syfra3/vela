Feature: Vela clustering installation support
  Vela users need a deliberate way to install baseline clustering dependencies
  so graph rebuilds can produce community metadata without late dependency or
  virtual-environment permission failures.

  @SCN-001 @REQ-001 @REQ-006
  Scenario: Install baseline clustering into a new repo-local virtual environment
    Given no repo-local ".venv" exists
    When the user runs "vela install --clustering"
    Then Vela creates a repo-local ".venv"
    And Vela installs dependencies from "requirements-clustering.txt"
    And Vela verifies that "networkx" imports from the selected environment
    And Vela reports clustering installation success

  @SCN-002 @REQ-001 @REQ-006
  Scenario: Reuse an existing writable repo-local virtual environment
    Given a repo-local ".venv" exists and is writable by the current user
    When the user runs "vela install --clustering"
    Then Vela uses the existing repo-local ".venv"
    And Vela installs dependencies from "requirements-clustering.txt"
    And Vela verifies that "networkx" imports from that environment
    And Vela reports clustering installation success

  @SCN-003 @REQ-002 @REQ-006
  Scenario: Fail clearly when the existing virtual environment is not writable
    Given a repo-local ".venv" exists
    And the current user cannot write required files inside ".venv"
    When the user runs "vela install --clustering"
    Then Vela fails before dependency installation
    And Vela identifies the repo-local ".venv" as not writable
    And Vela recommends rerunning with "--repair-venv" or applying documented manual remediation
    And Vela does not automatically repair permissions

  @SCN-004 @REQ-003 @REQ-006
  Scenario: Explicit repair mode makes the repo-local virtual environment writable before installing
    Given a repo-local ".venv" exists
    And the current user cannot write required files inside ".venv"
    When the user runs "vela install --clustering --repair-venv"
    Then Vela repairs writability only for the repo-local ".venv"
    And Vela reports that permission repair was performed
    And Vela installs dependencies from "requirements-clustering.txt"
    And Vela verifies that "networkx" imports from the selected environment
    And Vela reports clustering installation success

  @SCN-005 @REQ-003 @REQ-006
  Scenario: Explicit repair mode reports unrecoverable permission failures
    Given a repo-local ".venv" exists
    And the current user cannot make it writable
    When the user runs "vela install --clustering --repair-venv"
    Then Vela fails with the underlying permission error details
    And Vela does not report clustering installation success
    And Vela does not claim dependencies were installed

  @SCN-006 @REQ-004
  Scenario: Baseline clustering install does not install optional Leiden dependencies
    Given optional Leiden dependencies are not installed
    When the user runs "vela install --clustering"
    Then Vela installs baseline clustering dependencies from "requirements-clustering.txt"
    And Vela does not install "graspologic" by default
    And Vela keeps optional Leiden support separate from baseline clustering support

  @SCN-007 @REQ-004
  Scenario: Existing optional Leiden dependencies are preserved
    Given optional Leiden dependencies are already installed in the selected environment
    When the user runs "vela install --clustering"
    Then Vela installs or verifies baseline clustering dependencies
    And Vela does not remove existing optional Leiden dependencies
    And Vela does not downgrade optional Leiden dependencies solely because baseline clustering was requested

  @SCN-008 @REQ-005 @REQ-006
  Scenario: Successful install warns that an existing graph cache may lack community metadata
    Given an existing graph cache was built before baseline clustering was available
    When the user runs "vela install --clustering"
    Then Vela reports clustering installation success
    And Vela warns that existing graph cache may have been built without community metadata
    And Vela recommends a non-destructive graph rebuild command
    And Vela does not delete graph cache artifacts

  @SCN-009 @REQ-005 @REQ-006
  Scenario: No cache warning is needed when no graph cache exists
    Given no graph cache exists for the current workspace
    When the user runs "vela install --clustering"
    Then Vela reports clustering installation success
    And Vela reports that the next graph build can use clustering
    And Vela does not recommend deleting graph cache artifacts

  @SCN-010 @REQ-005 @REQ-006
  Scenario: Explicit force rebuild separates install success from rebuild failure
    Given a graph cache exists
    And the user explicitly requests force rebuild behavior during clustering install
    When dependency installation and "networkx" verification succeed
    But the graph rebuild fails
    Then Vela reports clustering installation success
    And Vela reports graph rebuild failure details separately
    And Vela does not roll back the installed clustering dependencies

  @SCN-011 @REQ-002 @REQ-006
  Scenario: Permission-denied venv creation is translated into an actionable Vela error
    Given a repo-local ".venv" exists with read-only activation files
    When Vela attempts to prepare the environment for "vela install --clustering"
    And the operating system returns a permission-denied error
    Then Vela reports that the repo-local ".venv" is not writable
    And Vela includes the relevant underlying permission-denied detail
    And Vela recommends explicit repair rather than silently changing permissions

  @SCN-012 @REQ-001 @REQ-006
  Scenario: Networkx verification failure prevents partial success
    Given dependency installation from "requirements-clustering.txt" has completed
    But "networkx" cannot be imported from the selected environment
    When Vela completes "vela install --clustering"
    Then Vela exits with failure
    And Vela reports that clustering verification failed
    And Vela does not report clustering installation success
