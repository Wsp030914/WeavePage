import { useContext } from 'react';
import { ProjectContext } from './project-context';

export default function useProjects() {
    return useContext(ProjectContext);
}
