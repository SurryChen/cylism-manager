## ADDED Requirements

### Requirement: Shared select menu preserves business selection semantics

The frontend SHALL provide `SelectMenu` as the shared single-value selection component. It MUST accept structured options and preserve the selected option's original value type when it emits `update:modelValue` and `change`.

#### Scenario: Select a numeric option

- **WHEN** a view binds a numeric field with `v-model.number` and the user selects an option whose value is numeric
- **THEN** the bound field and the emitted change value remain numeric rather than becoming a string

#### Scenario: Select an option from a dynamic list

- **WHEN** a view supplies options generated from loaded domain data and the user chooses one
- **THEN** the component emits that option's original value and closes the option list

#### Scenario: Render a contextual option

- **WHEN** a view supplies an option with a primary label and supplementary description
- **THEN** the option list displays both text values while the trigger may display its configured compact label

### Requirement: Shared select menu preserves form and accessibility contracts

`SelectMenu` SHALL support the current selection-control contract for placeholder text, disabled state, required form validation, identifier association, accessible name, and test attributes. Its visible trigger MUST expose listbox state and support opening with Enter, Space, or ArrowDown and closing with Escape or an outside click.

#### Scenario: Required selection is empty

- **WHEN** a required `SelectMenu` has no selected value and its enclosing form is submitted
- **THEN** the browser's form validity state rejects the submission until a value is selected

#### Scenario: Disabled selection is rendered

- **WHEN** a view renders `SelectMenu` as disabled
- **THEN** its trigger cannot open or change the selection and its disabled state is exposed to assistive technology

#### Scenario: Label and test hook target a select menu

- **WHEN** a view supplies an explicit identifier or `data-testid` to `SelectMenu`
- **THEN** the visible trigger remains label-addressable and the component remains discoverable through the supplied test hook

### Requirement: Business views use the shared select menu

All business selection controls under `web/src/views` SHALL render `SelectMenu` instead of directly rendering a native `<select>`. Migration MUST preserve each view's option labels, initial value, placeholder, disabled condition, change handler, and downstream request payload.

#### Scenario: A filter selection changes

- **WHEN** a user changes a namespace, status, project, environment, or other view filter
- **THEN** the view applies the same filtering behavior and request parameters as before the migration

#### Scenario: A form selection changes

- **WHEN** a user changes a form field such as a node, certificate, storage class, runtime, or application setting
- **THEN** the form retains the same validation and submits the same field value and type as before the migration

#### Scenario: Source templates are audited

- **WHEN** the frontend source is checked after the migration
- **THEN** `web/src/views` contains no direct native `<select>` element and the only such element under `web/src` is the internal compatibility control in `SelectMenu`
