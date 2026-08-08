import type { ReactNode } from 'react';
import { PlusCircle } from 'lucide-react';
import {
  getYamlToolboxSpec,
  type YamlEditorResourceKind,
  type YamlToolboxParameter,
  type YamlToolboxSnippet,
} from './yamlToolboxModel';

type YamlEditorToolboxProps = {
  resourceKind: YamlEditorResourceKind;
  suggestionSlot?: ReactNode;
  onInsertSnippet: (snippet: string) => void;
};

export function YamlEditorToolbox({
  resourceKind,
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
          {spec.parameterGroups.map(group => (
            <details key={group.id} className="yaml-toolbox-details">
              <summary>{group.title}</summary>
              <p>{group.description}</p>
              <div className="yaml-toolbox-param-list">
                {group.parameters.map(parameter => (
                  <ParameterDetails key={`${group.id}-${parameter.key}`} parameter={parameter} />
                ))}
              </div>
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
            <details key={group.id} className="yaml-toolbox-details">
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

function ParameterDetails({ parameter }: { parameter: YamlToolboxParameter }) {
  const hasMetadata = Boolean(parameter.valueHint || parameter.validValues?.length || parameter.structure);

  return (
    <details className="yaml-toolbox-param">
      <summary title={`${parameter.key}: ${parameter.description}`}>
        <code>{parameter.key}</code>
        <span>{parameter.description}</span>
      </summary>
      <div className="yaml-toolbox-param__body">
        {parameter.valueHint ? (
          <div>
            <span>Value</span>
            <code>{parameter.valueHint}</code>
          </div>
        ) : null}
        {parameter.validValues?.length ? (
          <div>
            <span>Values</span>
            <span className="yaml-toolbox-value-row">
              {parameter.validValues.map(value => (
                <code key={`${parameter.key}-${value}`}>{value}</code>
              ))}
            </span>
          </div>
        ) : null}
        {parameter.structure ? (
          <div>
            <span>Structure</span>
            <pre>
              <code>{parameter.structure}</code>
            </pre>
          </div>
        ) : null}
        {!hasMetadata ? <p>No constrained values or nested structure.</p> : null}
      </div>
    </details>
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
