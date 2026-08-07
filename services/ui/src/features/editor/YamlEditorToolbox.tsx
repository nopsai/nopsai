import type { ReactNode } from 'react';
import { PlusCircle } from 'lucide-react';
import { YamlValidationPanel, type YamlValidationError } from './YamlValidationPanel';
import {
  getYamlToolboxSpec,
  type YamlEditorResourceKind,
  type YamlToolboxSnippet,
} from './yamlToolboxModel';

type YamlEditorToolboxProps = {
  resourceKind: YamlEditorResourceKind;
  validationId: string;
  validationErrors: YamlValidationError[];
  validationMaxVisible?: number;
  invalidLabel?: string;
  renderValidationExample?: (message: string) => ReactNode;
  suggestionSlot?: ReactNode;
  onInsertSnippet: (snippet: string) => void;
};

export function YamlEditorToolbox({
  resourceKind,
  validationId,
  validationErrors,
  validationMaxVisible = 5,
  invalidLabel = 'Validation issues',
  renderValidationExample,
  suggestionSlot,
  onInsertSnippet,
}: YamlEditorToolboxProps) {
  const spec = getYamlToolboxSpec(resourceKind);

  return (
    <aside className="yaml-editor-toolbox" aria-label={spec.title}>
      <div className="yaml-editor-toolbox__header">
        <p className="yaml-editor-toolbox__kicker">Expanded editor</p>
        <h3>{spec.title}</h3>
      </div>

      <YamlValidationPanel
        id={validationId}
        errors={validationErrors}
        maxVisible={validationMaxVisible}
        invalidLabel={invalidLabel}
        inline
        renderExample={renderValidationExample}
      />

      {validationErrors.length > 0 ? (
        <div className="yaml-editor-toolbox__notice">
          {spec.invalidHint}
        </div>
      ) : null}

      {suggestionSlot ? (
        <section className="yaml-editor-toolbox__section" aria-label="Autocomplete suggestions">
          {suggestionSlot}
        </section>
      ) : null}

      <section className="yaml-editor-toolbox__section" aria-label="YAML parameters">
        <div className="yaml-editor-toolbox__section-heading">
          <h4>Parameters</h4>
        </div>
        <div className="yaml-editor-toolbox__details-list">
          {spec.parameterGroups.map((group, index) => (
            <details key={group.id} className="yaml-toolbox-details" open={index === 0 || validationErrors.length > 0}>
              <summary>{group.title}</summary>
              <p>{group.description}</p>
              <dl className="yaml-toolbox-param-list">
                {group.parameters.map(parameter => (
                  <div key={`${group.id}-${parameter.key}`} className="yaml-toolbox-param">
                    <dt>{parameter.key}</dt>
                    <dd>
                      <span>{parameter.description}</span>
                      {parameter.valueHint ? <code>{parameter.valueHint}</code> : null}
                      {parameter.validValues?.length ? (
                        <span className="yaml-toolbox-value-row">
                          {parameter.validValues.map(value => (
                            <code key={`${parameter.key}-${value}`}>{value}</code>
                          ))}
                        </span>
                      ) : null}
                      {parameter.structure ? (
                        <pre>
                          <code>{parameter.structure}</code>
                        </pre>
                      ) : null}
                    </dd>
                  </div>
                ))}
              </dl>
            </details>
          ))}
        </div>
      </section>

      <section className="yaml-editor-toolbox__section" aria-label="YAML insertion toolbox">
        <div className="yaml-editor-toolbox__section-heading">
          <h4>Insert Structure</h4>
        </div>
        <div className="yaml-editor-toolbox__details-list">
          {spec.snippetGroups.map(group => (
            <details key={group.id} className="yaml-toolbox-details" open>
              <summary>{group.title}</summary>
              <p>{group.description}</p>
              <div className="yaml-toolbox-snippet-grid">
                {group.snippets.map(snippet => (
                  <SnippetButton
                    key={snippet.id}
                    snippet={snippet}
                    onInsertSnippet={onInsertSnippet}
                  />
                ))}
              </div>
            </details>
          ))}
        </div>
      </section>

      <section className="yaml-editor-toolbox__section" aria-label="Light YAML samples">
        <div className="yaml-editor-toolbox__section-heading">
          <h4>Samples</h4>
        </div>
        <div className="yaml-editor-toolbox__details-list">
          {spec.samples.map(sample => (
            <details key={sample.title} className="yaml-toolbox-details yaml-toolbox-details--sample">
              <summary>{sample.title}</summary>
              <pre>
                <code>{sample.yaml}</code>
              </pre>
            </details>
          ))}
        </div>
      </section>
    </aside>
  );
}

function SnippetButton({
  snippet,
  onInsertSnippet,
}: {
  snippet: YamlToolboxSnippet;
  onInsertSnippet: (snippet: string) => void;
}) {
  return (
    <button
      type="button"
      className="yaml-toolbox-snippet"
      onMouseDown={event => event.preventDefault()}
      onClick={() => onInsertSnippet(snippet.yaml)}
      title={snippet.description}
    >
      <PlusCircle className="h-4 w-4" aria-hidden="true" />
      <span>
        <strong>{snippet.label}</strong>
        <small>{snippet.description}</small>
      </span>
    </button>
  );
}
