import client from './client';

export const createDocumentImportSession = async (data) => {
    return client.post('/documents/imports', data, { timeout: 20000 });
};

export const uploadDocumentImportPart = async (uploadId, partNo, blob) => {
    return client.put(`/documents/imports/${uploadId}/parts/${partNo}`, blob, {
        headers: { 'Content-Type': 'application/octet-stream' },
        timeout: 30000,
    });
};

export const uploadDocumentImportAsset = async (uploadId, file, originalPath) => {
    const form = new FormData();
    form.append('file', file);
    if (originalPath) {
        form.append('original_path', originalPath);
    }

    return client.post(`/documents/imports/${uploadId}/assets`, form, {
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 30000,
    });
};

export const completeDocumentImport = async (uploadId, data = {}) => {
    return client.post(`/documents/imports/${uploadId}/complete`, data, { timeout: 30000 });
};

export const abortDocumentImport = async (uploadId) => {
    return client.delete(`/documents/imports/${uploadId}`, { timeout: 10000 });
};
