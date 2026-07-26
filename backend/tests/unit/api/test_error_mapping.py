from api.errors import to_problem
from services.render_snapshots import RenderInputInvalid


def test_render_input_error_uses_global_problem_mapping() -> None:
    problem = to_problem(RenderInputInvalid("missing shot"))

    assert problem.status == 422
    assert problem.code == "RENDER_INPUT_INVALID"
    assert problem.detail == "missing shot"
