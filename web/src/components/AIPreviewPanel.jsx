import React from 'react';
import Button from './Button';
import './AIPreviewPanel.css';

function trimPreview(value) {
    return String(value || '').trim();
}

export default function AIPreviewPanel({
    busy = false,
    mode = '',
    error = '',
    instruction = '',
    selectedText = '',
    previewText = '',
    meetingPreview = null,
    canGenerateMeeting = false,
    onInstructionChange,
    onGenerateDraft,
    onGenerateContinue,
    onGenerateMeeting,
    onCancel,
    onApplyInsert,
    onApplyReplaceSelection,
    onApplyReplaceAll,
    onUseMeetingAction,
}) {
    const hasTextPreview = trimPreview(previewText) !== '';
    const hasMeetingPreview = Boolean(meetingPreview);

    return (
        <div className="yq-ai-preview-panel">
            <div className="yq-ai-preview-header">
                <div>
                    <h3>AI Workspace</h3>
                    <p>Generate preview content first, then apply it manually.</p>
                </div>
                {busy ? (
                    <Button variant="secondary" onClick={onCancel}>
                        Cancel
                    </Button>
                ) : null}
            </div>

            <div className="yq-ai-preview-controls">
                <label className="yq-input-label" htmlFor="ai-instruction">Instruction</label>
                <textarea
                    id="ai-instruction"
                    className="yq-ai-preview-textarea"
                    value={instruction}
                    onChange={(event) => onInstructionChange?.(event.target.value)}
                    placeholder="例如：扩写这段内容、改成更专业的口吻、整理成会议纪要"
                    rows={3}
                    disabled={busy}
                />
                <div className="yq-ai-preview-action-row">
                    <Button variant="secondary" onClick={onGenerateDraft} disabled={busy}>
                        {busy && mode === 'draft' ? 'Generating...' : 'Generate Draft'}
                    </Button>
                    <Button variant="secondary" onClick={onGenerateContinue} disabled={busy}>
                        {busy && mode === 'continue' ? 'Generating...' : 'Continue / Rewrite'}
                    </Button>
                    {canGenerateMeeting ? (
                        <Button variant="secondary" onClick={onGenerateMeeting} disabled={busy}>
                            {busy && mode === 'meeting' ? 'Generating...' : 'Meeting Preview'}
                        </Button>
                    ) : null}
                </div>
                {trimPreview(selectedText) ? (
                    <div className="yq-ai-selected-text">
                        <strong>Selected text</strong>
                        <p>{selectedText}</p>
                    </div>
                ) : null}
                {error ? <div className="yq-ai-preview-error">{error}</div> : null}
            </div>

            {hasTextPreview ? (
                <div className="yq-ai-preview-result">
                    <div className="yq-ai-preview-result-header">
                        <h4>Preview Result</h4>
                        <div className="yq-ai-preview-action-row">
                            <Button variant="secondary" onClick={onApplyInsert} disabled={busy}>
                                Insert
                            </Button>
                            <Button variant="secondary" onClick={onApplyReplaceSelection} disabled={busy}>
                                Replace Selection
                            </Button>
                            <Button variant="secondary" onClick={onApplyReplaceAll} disabled={busy}>
                                Replace All
                            </Button>
                        </div>
                    </div>
                    <pre className="yq-ai-preview-code">{previewText}</pre>
                </div>
            ) : null}

            {hasMeetingPreview ? (
                <div className="yq-ai-preview-result">
                    <div className="yq-ai-preview-result-header">
                        <h4>Meeting Preview</h4>
                        <div className="yq-ai-preview-action-row">
                            <Button variant="secondary" onClick={onApplyInsert} disabled={busy}>
                                Insert Minutes
                            </Button>
                            <Button variant="secondary" onClick={onApplyReplaceAll} disabled={busy}>
                                Replace Content
                            </Button>
                        </div>
                    </div>

                    {trimPreview(meetingPreview.summary) ? (
                        <div className="yq-ai-preview-block">
                            <strong>Summary</strong>
                            <p>{meetingPreview.summary}</p>
                        </div>
                    ) : null}

                    {Array.isArray(meetingPreview.decisions) && meetingPreview.decisions.length > 0 ? (
                        <div className="yq-ai-preview-block">
                            <strong>Decisions</strong>
                            <ul>
                                {meetingPreview.decisions.map((decision, index) => (
                                    <li key={`${decision}-${index}`}>{decision}</li>
                                ))}
                            </ul>
                        </div>
                    ) : null}

                    {Array.isArray(meetingPreview.actions) && meetingPreview.actions.length > 0 ? (
                        <div className="yq-ai-preview-block">
                            <strong>Action Candidates</strong>
                            <div className="yq-ai-preview-action-list">
                                {meetingPreview.actions.map((action, index) => (
                                    <div key={`${action.title}-${index}`} className="yq-ai-preview-action-card">
                                        <div>
                                            <div className="yq-ai-preview-action-title">{action.title}</div>
                                            {(trimPreview(action.owner_hint) || trimPreview(action.due_hint)) ? (
                                                <div className="yq-ai-preview-action-meta">
                                                    {trimPreview(action.owner_hint) ? `Owner: ${action.owner_hint}` : 'Owner: TBD'}
                                                    {trimPreview(action.due_hint) ? ` · Due: ${action.due_hint}` : ''}
                                                </div>
                                            ) : null}
                                        </div>
                                        <Button variant="secondary" onClick={() => onUseMeetingAction?.(action)}>
                                            Use as todo
                                        </Button>
                                    </div>
                                ))}
                            </div>
                        </div>
                    ) : null}

                    {trimPreview(meetingPreview.minutes_markdown) ? (
                        <div className="yq-ai-preview-block">
                            <strong>Minutes Markdown</strong>
                            <pre className="yq-ai-preview-code">{meetingPreview.minutes_markdown}</pre>
                        </div>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}
