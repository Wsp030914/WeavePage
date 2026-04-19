import React, { useEffect, useRef } from 'react';
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

export default function DocumentMarkdownEditor({ provider, value = '', onChange }) {
    const hostRef = useRef(null);
    const viewRef = useRef(null);
    const applyingRemoteRef = useRef(false);
    const initialValueRef = useRef(value);

    useEffect(() => {
        const yText = provider?.getText?.();
        if (!hostRef.current || !yText) return undefined;

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
            ],
        });
        viewRef.current = view;

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
    }, [provider, onChange]);

    if (!provider) {
        return (
            <div className="yq-document-editor-shell is-loading">
                Loading collaborative editor...
            </div>
        );
    }

    return <div ref={hostRef} className="yq-document-editor-shell" />;
}
