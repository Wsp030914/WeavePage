import React, { useCallback, useEffect, useRef } from 'react';
import { markdown } from '@codemirror/lang-markdown';
import { basicSetup, EditorView } from 'codemirror';
import './DocumentMarkdownEditor.css';

function yEventToEditorChanges(event) {
    const changes = [];
    let index = 0;

    event.changes.delta.forEach((delta) => {
        if (delta.retain) {
            index += delta.retain;
        }
        if (delta.delete) {
            changes.push({ from: index, to: index + delta.delete, insert: '' });
        }
        if (delta.insert) {
            const insertText = String(delta.insert);
            changes.push({ from: index, to: index, insert: insertText });
            index += insertText.length;
        }
    });

    return changes;
}

function applyEditorChangesToYText(yText, changes) {
    let offset = 0;

    changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
        const index = fromA + offset;
        const deleteLength = toA - fromA;
        const insertText = inserted.toString();

        if (deleteLength > 0) {
            yText.delete(index, deleteLength);
        }
        if (insertText) {
            yText.insert(index, insertText);
        }
        offset += insertText.length - deleteLength;
    });
}

export default function DocumentMarkdownEditor({
    provider,
    value = '',
    onChange,
    collaborative = true,
    onSelectionChange,
    command = null,
}) {
    const hostRef = useRef(null);
    const viewRef = useRef(null);
    const applyingRemoteRef = useRef(false);
    const initialValueRef = useRef(value);
    const lastCommandIDRef = useRef('');

    const emitSelection = useCallback((view) => {
        const range = view.state.selection.main;
        onSelectionChange?.({
            from: range.from,
            to: range.to,
            text: view.state.sliceDoc(range.from, range.to),
        });
    }, [onSelectionChange]);

    useEffect(() => {
        if (!hostRef.current) return undefined;

        if (!collaborative) {
            const view = new EditorView({
                parent: hostRef.current,
                doc: initialValueRef.current || '',
                extensions: [
                    basicSetup,
                    markdown(),
                    EditorView.lineWrapping,
                    EditorView.updateListener.of((update) => {
                        if (update.docChanged) {
                            onChange?.(update.state.doc.toString());
                        }
                        if (update.docChanged || update.selectionSet) {
                            emitSelection(update.view);
                        }
                    }),
                ],
            });
            viewRef.current = view;
            emitSelection(view);

            return () => {
                view.destroy();
                if (viewRef.current === view) {
                    viewRef.current = null;
                }
            };
        }

        const yText = provider?.getText?.();
        if (!yText) return undefined;

        const view = new EditorView({
            parent: hostRef.current,
            doc: yText.toString() || initialValueRef.current || '',
            extensions: [
                basicSetup,
                markdown(),
                EditorView.lineWrapping,
                EditorView.updateListener.of((update) => {
                    if (!update.docChanged || applyingRemoteRef.current) return;

                    provider.getDoc().transact(() => {
                        applyEditorChangesToYText(yText, update.changes);
                    }, 'codemirror-editor');
                }),
                EditorView.updateListener.of((update) => {
                    if (update.docChanged || update.selectionSet) {
                        emitSelection(update.view);
                    }
                }),
            ],
        });
        viewRef.current = view;
        emitSelection(view);

        const observer = (event) => {
            const changes = yEventToEditorChanges(event);
            if (changes.length === 0) {
                onChange?.(yText.toString());
                return;
            }

            applyingRemoteRef.current = true;
            try {
                view.dispatch({ changes });
            } finally {
                applyingRemoteRef.current = false;
            }
            onChange?.(yText.toString());
            emitSelection(view);
        };

        yText.observe(observer);
        onChange?.(yText.toString());

        return () => {
            yText.unobserve(observer);
            view.destroy();
            if (viewRef.current === view) {
                viewRef.current = null;
            }
        };
    }, [provider, onChange, collaborative, emitSelection]);

    useEffect(() => {
        if (!command || !command.id || lastCommandIDRef.current === command.id) return;
        const view = viewRef.current;
        if (!view) return;

        const selection = view.state.selection.main;
        let from = typeof command.from === 'number' ? command.from : selection.from;
        let to = typeof command.to === 'number' ? command.to : selection.to;
        const docLength = view.state.doc.length;
        from = Math.max(0, Math.min(from, docLength));
        to = Math.max(from, Math.min(to, docLength));

        if (command.type === 'replace_all') {
            view.dispatch({
                changes: { from: 0, to: docLength, insert: command.text || '' },
                selection: { anchor: String(command.text || '').length },
            });
        } else if (command.type === 'replace_selection') {
            view.dispatch({
                changes: { from, to, insert: command.text || '' },
                selection: { anchor: from + String(command.text || '').length },
            });
        } else if (command.type === 'insert_after_selection') {
            view.dispatch({
                changes: { from: to, to, insert: command.text || '' },
                selection: { anchor: to + String(command.text || '').length },
            });
        }

        emitSelection(view);
        lastCommandIDRef.current = command.id;
    }, [command, emitSelection]);

    if (collaborative && !provider) {
        return (
            <div className="yq-document-editor-shell is-loading">
                Loading collaborative editor...
            </div>
        );
    }

    return <div ref={hostRef} className="yq-document-editor-shell" />;
}
